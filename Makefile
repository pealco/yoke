BINARY := yoke
CMD := ./cmd/yoke
DIST := dist

PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

GOLANGCI_LINT_VERSION ?= v2.8.0
GOVULNCHECK_VERSION ?= v1.1.4
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

.PHONY: build install clean release fmt fmt-check lint test test-race typecheck vet vuln mod-tidy mod-check check ci

build:
	mkdir -p $(DIST)
	go build -o $(DIST)/$(BINARY) $(CMD)

install:
	go install $(CMD)

clean:
	rm -rf $(DIST)

release:
	mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUT="$(DIST)/$(BINARY)_$${OS}_$${ARCH}"; \
		echo "building $$OUT"; \
		GOOS=$$OS GOARCH=$$ARCH go build -o $$OUT $(CMD); \
	done

fmt:
	$(GOLANGCI_LINT) fmt ./...

fmt-check:
	@DIFF="$$( $(GOLANGCI_LINT) fmt --diff ./... )"; \
	if [ -n "$$DIFF" ]; then \
		printf "%s\n" "$$DIFF"; \
		echo "Formatting drift detected. Run 'make fmt'."; \
		exit 1; \
	fi

lint:
	$(GOLANGCI_LINT) run ./...

test:
	go test ./...

test-race:
	go test -race ./...

typecheck:
	go test -run=^$$ ./...

vet:
	go vet ./...

vuln:
	$(GOVULNCHECK) ./...

mod-tidy:
	go mod tidy

mod-check:
	go mod tidy -diff

check: fmt-check mod-check typecheck vet lint test vuln

ci: check
