# Makefile for Go project

default: build

# All target
all: ui
	$(MAKE) build

ui: ui-admin ui-user

ui-admin:
	cd ui/admin && \
	pnpm install && \
	pnpm run build

ui-user:
	cd ui/user && \
	pnpm install && \
	pnpm run build

clean:
	rm -rf ui/admin/build
	rm -rf ui/user/build

serve-docs:
	cd docs && \
	npm install && \
	npm run start

# Build the project

GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null | xargs -I {} echo -X 'github.com/obot-platform/obot/pkg/version.Tag={}')
GO_LD_FLAGS := "-s -w $(GIT_TAG)"
build:
	go build -ldflags=$(GO_LD_FLAGS) -o bin/obot .

dev:
	./tools/dev.sh $(ARGS)

dev-open: ARGS=--open-uis
dev-open: dev

# Lint the project
lint: lint-admin lint-go

lint-admin:
	cd ui/admin && \
	pnpm run format && \
	pnpm run lint

package-tools:
	./tools/package-tools.sh

tidy:
	go mod tidy

GOLANGCI_LINT_VERSION ?= v1.64.5
setup-env:
	if ! command -v golangci-lint &> /dev/null; then \
  		echo "Could not find golangci-lint, installing version $(GOLANGCI_LINT_VERSION)."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION); \
	fi

lint-go: setup-env
	golangci-lint run

generate:
	go generate

# Runs Go linters and validates that all generated code is committed.
validate-go-code: tidy generate lint-go no-changes

no-changes:
	@if [ -n "$$(git status --porcelain)" ]; then \
		git status --porcelain; \
		git --no-pager diff; \
		echo "Encountered dirty repo!"; \
		exit 1; \
	fi

.PHONY: ui ui-admin ui-user build all clean dev dev-open lint lint-admin lint-api no-changes fmt tidy
