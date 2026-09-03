VERSION ?= 0.3.0
LDFLAGS := -s -w -X main.version=$(VERSION)
GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: build fmt fmt-check test race lint security vuln smoke quality verify install release clean sync-models

build:
	go build -ldflags "$(LDFLAGS)" -o bin/omi ./cmd/omi

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	@test -z "$$(gofmt -l $(GOFILES))" || (gofmt -l $(GOFILES); echo "run: make fmt" >&2; exit 1)

test:
	go test ./...

race:
	go test -race ./...

lint: fmt-check
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run

security:
	go run github.com/securego/gosec/v2/cmd/gosec@v2.29.0 ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...

smoke: build
	BIN=./bin/omi scripts/smoke.sh

quality: fmt-check test race lint

verify: quality security vuln build

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/omi

release:
	@echo "Run: make verify"
	@echo "Build artifacts: goreleaser release --snapshot --clean (local) or git tag v$(VERSION) && git push origin v$(VERSION)"
	@echo "GitHub Actions release.yml will build + publish tagged releases."

clean:
	rm -rf bin/ dist/

sync-models:
	scripts/sync-models.sh --write-snapshots
