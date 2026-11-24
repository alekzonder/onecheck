VERSION ?= dev
PKG := github.com/alekzonder/onecheck
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION)
MODULES := $(patsubst cmd/onecheck-%,%,$(wildcard cmd/onecheck-*))
PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
GOLANGCI_VERSION := v2.15.0
GOLANGCI := $(CURDIR)/bin/golangci-lint

export CGO_ENABLED ?= 0

.PHONY: help build test fmt fmt-check vet lint check release-assets docs-build docs clean

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

build: ## Build onecheck and bundled modules into dist/
	go build -ldflags '$(LDFLAGS)' -o dist/onecheck ./cmd/onecheck
	@for m in $(MODULES); do \
		echo "go build onecheck-$$m"; \
		go build -ldflags '$(LDFLAGS)' -o dist/modules/onecheck-$$m ./cmd/onecheck-$$m || exit 1; \
	done

test: ## Run tests with the race detector
	CGO_ENABLED=1 go test -race ./...

fmt: ## Format the code
	gofmt -w .

fmt-check: ## Fail if any file is not gofmt-formatted
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not formatted:"; echo "$$out"; exit 1; fi

vet: ## Run go vet
	go vet ./...

$(GOLANGCI):
	GOBIN=$(CURDIR)/bin CGO_ENABLED=1 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

lint: $(GOLANGCI) ## Run golangci-lint (includes cyclomatic complexity <= 25)
	$(GOLANGCI) run ./...

check: fmt-check vet lint test ## Run every check; must pass before finishing work

release-assets: ## Cross-build release assets and checksums into dist/release/
	@rm -rf dist/release && mkdir -p dist/release
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "build $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' -o dist/release/onecheck_$${os}_$${arch} ./cmd/onecheck || exit 1; \
		for m in $(MODULES); do \
			GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' -o dist/release/onecheck-$${m}_$${os}_$${arch} ./cmd/onecheck-$$m || exit 1; \
		done; \
	done
	@cd dist/release && sha256sum onecheck* > checksums.txt

docs-build: ## Check links and build the documentation site into docs/dist/
	scripts/docs-check-links-test.sh
	scripts/docs-check-links.sh
	@if [ ! -d docs/node_modules ]; then cd docs && npm ci; fi
	cd docs && npm run doctor && npm run build

docs: ## Build the documentation site and publish it to branch docs
	scripts/publish-docs.sh

clean: ## Remove build output
	rm -rf dist
