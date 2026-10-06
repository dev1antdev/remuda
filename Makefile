.PHONY: all build web server dev run test typecheck docker docker-test release deploy plugin clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

all: build

web/node_modules: web/package.json
	cd web && npm install

web: web/node_modules
	cd web && npm run build

build: web
	go build -ldflags "-X main.version=$(VERSION)" -o bin/herdr-ui ./cmd/herdr-ui

run: build
	./bin/herdr-ui

# Development: `make server` in one terminal, `make dev` in another (Vite on :5173).
server:
	go run ./cmd/herdr-ui

dev: web/node_modules
	cd web && npm run dev

test:
	go vet ./...
	go test ./...

typecheck: web/node_modules
	cd web && npm run typecheck
	go vet ./...

docker:
	docker build --target runtime --build-arg VERSION=$(VERSION) -t herdr-ui:local .

docker-test:
	docker build --target test -t herdr-ui:test .

# Binaries for every host you run herdr on (the SPA is embedded, so one file each).
release: web
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" \
	    -o bin/herdr-ui-$$os-$$arch ./cmd/herdr-ui; \
	done

# make deploy HOST=gog.remote — puts the right binary into ~/.local/bin on that host.
deploy: release
	@test -n "$(HOST)" || (echo "usage: make deploy HOST=<ssh host>"; exit 1)
	@t=$$(ssh $(HOST) 'uname -sm' | tr 'A-Z' 'a-z' | sed 's/x86_64/amd64/; s/aarch64/arm64/; s/ /-/'); \
	  echo "deploying bin/herdr-ui-$$t to $(HOST):~/.local/bin/herdr-ui"; \
	  ssh $(HOST) 'mkdir -p ~/.local/bin' && scp -q bin/herdr-ui-$$t $(HOST):~/.local/bin/herdr-ui && \
	  ssh $(HOST) '~/.local/bin/herdr-ui -version'

# The JetBrains IDE plugin (plugin/build/distributions/*.zip).
plugin:
	cd plugin && ./gradlew buildPlugin -q

clean:
	rm -rf bin web/dist plugin/build
