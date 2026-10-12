.PHONY: all build web server dev run test typecheck docker docker-test release publish deploy service code clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

all: build

web/node_modules: web/package.json
	cd web && npm install

web: web/node_modules
	cd web && REMUDA_VERSION=$(VERSION) npm run build

build: web
	go build -ldflags "-X main.version=$(VERSION)" -o bin/remuda ./cmd/remuda

run: build
	./bin/remuda

# Development: `make server` in one terminal, `make dev` in another (Vite on :5173).
server:
	go run ./cmd/remuda

dev: web/node_modules
	cd web && npm run dev

test:
	go vet ./...
	go test ./...

typecheck: web/node_modules
	cd web && npm run typecheck
	go vet ./...

docker:
	docker build --target runtime --build-arg VERSION=$(VERSION) -t remuda:local .

docker-test:
	docker build --target test -t remuda:test .

# Binaries for every host you run herdr on (the SPA is embedded, so one file
# each), and their sha256 sums — what a machine checks a release download
# against (internal/health/release.go).
release: web
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" \
	    -o bin/remuda-$$os-$$arch ./cmd/remuda; \
	done
	@cd bin && shasum -a 256 remuda-darwin-arm64 remuda-darwin-amd64 remuda-linux-amd64 remuda-linux-arm64 > checksums.txt && echo "bin/checksums.txt written"

# make publish — a release: the tag on HEAD is pushed, and the Release
# workflow (.github/workflows/release.yml) builds the four binaries and
# checksums.txt there and makes the GitHub release — with install.sh, the
# one-line installer — that every remuda out there then offers as
# "Download & install". Tag first:
#   git tag -a vX.Y.Z -m 'remuda X.Y.Z'
publish:
	@git describe --tags --exact-match >/dev/null 2>&1 || { echo "publish needs a tag on HEAD: git tag -a vX.Y.Z -m 'remuda X.Y.Z'"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "the tree is not clean: commit first"; exit 1; }
	@$(MAKE) test typecheck
	git push origin HEAD $(VERSION)
	@echo "the Release workflow builds and publishes $(VERSION): gh run watch, or https://github.com/dev1antdev/remuda/actions"

# make deploy HOST=studio.remote — puts the right binary into ~/.local/bin on that host.
deploy: release
	@test -n "$(HOST)" || (echo "usage: make deploy HOST=<ssh host>"; exit 1)
	@t=$$(ssh $(HOST) 'uname -sm' | tr 'A-Z' 'a-z' | sed 's/x86_64/amd64/; s/aarch64/arm64/; s/ /-/'); \
	  echo "deploying bin/remuda-$$t to $(HOST):~/.local/bin/remuda"; \
	  ssh $(HOST) 'mkdir -p ~/.local/bin' && scp -q bin/remuda-$$t $(HOST):.local/bin/remuda.new && \
	  ssh $(HOST) 'mv ~/.local/bin/remuda.new ~/.local/bin/remuda && ~/.local/bin/remuda -version' # rename: safe while the old one runs

# make service HOST=studio.remote — remuda as a background service there (launchd / systemd --user),
# listening on 127.0.0.1:7317, starting at login. Without HOST: on this machine.
service:
	@if [ -n "$(HOST)" ]; then \
	  { printf 'IFS= read -r TS_AUTHKEY; export TS_AUTHKEY REMUDA_TAILSCALE=%s REMUDA_TAILSCALE_HOSTNAME=%s\n%s\n' "$(REMUDA_TAILSCALE)" "$(REMUDA_TAILSCALE_HOSTNAME)" "$(TS_AUTHKEY)"; cat internal/setup/scripts/install-service.sh; } | ssh $(HOST) 'sh -s install'; \
	else sh internal/setup/scripts/install-service.sh install; fi
	# the auth key goes over stdin, read by the script's first line: never on the ssh command line, where ps on either end would show it

# make code [HOST=studio.remote] — installs code-server (VS Code in the browser)
# as a per-user service on that host, bound to 127.0.0.1:7320 with no auth of
# its own; remuda proxies it under /code/<machine>/ behind its token. The
# binary carries the same script: `remuda code [-host …]`.
code:
	@if [ -n "$(HOST)" ]; then ssh $(HOST) 'sh -s' < internal/setup/scripts/install-code.sh; else sh internal/setup/scripts/install-code.sh; fi

clean:
	rm -rf bin web/dist
