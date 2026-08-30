.PHONY: build test lint generate run-server run-agent release clean bench \
	test-cover test-race test-smoke frontend test-ui gate

version ?= $$(git describe --tags --always 2>/dev/null || echo "dev")

# web/frontend/node_modules ships a stray Go package; every Go target must skip it.
GOPKGS = $(shell go list ./... | grep -v node_modules)

build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(version)" -o hopstat ./cmd/lg/

generate:
	sqlc generate

# -shuffle=on: tests that depend on run order (shared caches, package-level seams) pass
# under the default order and fail in CI. Shuffling makes that failure immediate.
test:
	go test -shuffle=on $(GOPKGS)

test-cover:
	go test -shuffle=on $(GOPKGS) -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/...
	go tool cover -func=coverage.out | tail -1
	@./scripts/coverage-gate.sh coverage.out

test-race:
	go test -race -shuffle=on $(GOPKGS)

test-smoke:
	go test -count=1 -tags=smoke ./tests/smoke/...

bench:
	go test -run=^$$ -bench=. -benchmem ./internal/engine/... ./internal/geo/... ./internal/store/querystore/...

lint:
	go vet $(GOPKGS)
	golangci-lint run ./internal/... ./cmd/...

frontend:
	cd web/frontend && npx tsc -b
	cd web/frontend && npx eslint . --max-warnings=0
	cd web/frontend && npm run test:cover
	cd web/frontend && npm run build

# Drives the production bundle in a real browser: pages render, and every run of text on
# them clears its WCAG threshold across brand colours and both themes.
test-ui:
	cd web/frontend && npx playwright test

# Everything CI enforces, in one command.
gate: lint test-cover test-race test-smoke frontend test-ui

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
