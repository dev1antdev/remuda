.PHONY: all build web server dev run test typecheck docker docker-test clean

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

clean:
	rm -rf bin web/dist
