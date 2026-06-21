.PHONY: build test lint generate run-server run-agent release clean bench test-cover test-race

version ?= $$(git describe --tags --always 2>/dev/null || echo "dev")

build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(version)" -o hopstat ./cmd/lg/

generate:
	sqlc generate

test:
	go test ./...

test-cover:
	go test $(shell go list ./... | grep -v node_modules) -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/...
	go tool cover -func=coverage.out | tail -1
	@pct=$$(go tool cover -func=coverage.out | tail -1 | sed -E 's/.*[^0-9]([0-9]+\.[0-9]+)%/\1/'); \
	awk -v p="$$pct" 'BEGIN { if (p+0 < 100) { printf "coverage %.1f%% below 100%% threshold\n", p; exit 1 } }'

test-race:
	go test -race ./...

bench:
	go test -run=^$$ -bench=. -benchmem ./internal/engine/... ./internal/geo/... ./internal/store/querystore/...

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
