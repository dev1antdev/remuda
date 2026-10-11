.PHONY: all build web server dev run test typecheck docker docker-test release deploy service code clean

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

# Binaries for every host you run herdr on (the SPA is embedded, so one file each).
release: web
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" \
	    -o bin/remuda-$$os-$$arch ./cmd/remuda; \
	done

# make deploy HOST=gog.remote — puts the right binary into ~/.local/bin on that host.
deploy: release
	@test -n "$(HOST)" || (echo "usage: make deploy HOST=<ssh host>"; exit 1)
	@t=$$(ssh $(HOST) 'uname -sm' | tr 'A-Z' 'a-z' | sed 's/x86_64/amd64/; s/aarch64/arm64/; s/ /-/'); \
	  echo "deploying bin/remuda-$$t to $(HOST):~/.local/bin/remuda"; \
	  ssh $(HOST) 'mkdir -p ~/.local/bin' && scp -q bin/remuda-$$t $(HOST):.local/bin/remuda.new && \
	  ssh $(HOST) 'mv ~/.local/bin/remuda.new ~/.local/bin/remuda && ~/.local/bin/remuda -version' # rename: safe while the old one runs

# make service HOST=gog.remote — remuda as a background service there (launchd / systemd --user),
# listening on 127.0.0.1:7317, starting at login. Without HOST: on this machine.
service:
	@if [ -n "$(HOST)" ]; then ssh $(HOST) 'REMUDA_TAILSCALE=$(REMUDA_TAILSCALE) REMUDA_TAILSCALE_HOSTNAME=$(REMUDA_TAILSCALE_HOSTNAME) TS_AUTHKEY=$(TS_AUTHKEY) sh -s install' < internal/setup/scripts/install-service.sh; else sh internal/setup/scripts/install-service.sh install; fi

# make code [HOST=gog.remote] — installs code-server (VS Code in the browser)
# as a per-user service on that host, bound to 127.0.0.1:7320 with no auth of
# its own; remuda proxies it under /code/<machine>/ behind its token.
code:
	@if [ -n "$(HOST)" ]; then ssh $(HOST) 'sh -s' < deploy/install-code.sh; else sh deploy/install-code.sh; fi

clean:
	rm -rf bin web/dist
