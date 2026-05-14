.PHONY: build test lint generate run-server run-agent release clean

version ?= $$(git describe --tags --always 2>/dev/null || echo "dev")

build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(version)" -o hopstat ./cmd/lg/

generate:
	sqlc generate

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run ./...

run-server:
	go run ./cmd/lg/ --mode=server --config=config.example.yaml

run-agent:
	go run ./cmd/lg/ --mode=agent --config=config.example.yaml

release:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(version)" -o dist/hopstat-linux-amd64 ./cmd/lg/
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(version)" -o dist/hopstat-linux-arm64 ./cmd/lg/
	cd dist && sha256sum hopstat-linux-amd64 hopstat-linux-arm64 > checksums.txt

clean:
	rm -f hopstat
	rm -rf dist/
