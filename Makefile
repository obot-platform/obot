# Makefile for Go project

default: build

# All target
all: ui
	$(MAKE) build

ui: ui-user ui-user-node

ui-user:
	cd ui/user && \
	pnpm install && \
	pnpm run build

ui-user-node:
	cd ui/user && \
	pnpm install && \
	BUILD=node pnpm run build

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
lint: lint-go

tidy:
	go mod tidy

GOLANGCI_LINT_VERSION ?= v2.9.0
setup-env:
	if ! command -v golangci-lint &> /dev/null; then \
  		echo "Could not find golangci-lint, installing version $(GOLANGCI_LINT_VERSION)."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION); \
	fi

lint-go: setup-env
	golangci-lint run

generate:
	go generate

test:
	go test -v -cover ./...

# Runs Go linters and validates that all generated code is committed.
validate-go-code: tidy generate lint-go no-changes

no-changes:
	@if [ -n "$$(git status --porcelain)" ]; then \
		git status --porcelain; \
		git --no-pager diff; \
		echo "Encountered dirty repo!"; \
		exit 1; \
	fi

#cut a new version for release with items in docs/docs
gen-docs-release:
	if [ -z ${version} ]; then \
  			echo "version not set (version=x.x)"; \
    		exit 1 \
    	;fi
	if [ -z ${prev_version} ]; then \
  			echo "prev_version not set (prev_version=x.x)"; \
    		exit 1 \
    	;fi
	docker run --rm --workdir=/docs -v $${PWD}/docs:/docs node:24-bookworm yarn docusaurus docs:version ${version}
	awk '/versions/&& ++c == 1 {print;print "\t\t\t\"${prev_version}\": {label: \"${prev_version}\", banner: \"none\", path: \"${prev_version}\"},";next}1' ./docs/docusaurus.config.ts > tmp.config.ts && mv tmp.config.ts ./docs/docusaurus.config.ts
	sed -i.bak "s/lastVersion: '[^']*'/lastVersion: '${version}'/" ./docs/docusaurus.config.ts && rm -f ./docs/docusaurus.config.ts.bak

# Completely remove doc version from docs site
remove-docs-version:
	if [ -z ${version} ]; then \
  			echo "version not set (version=x.x)"; \
    		exit 1 \
    	;fi
	echo "removing ${version} from documentation completely"
	-rm  "./docs/versioned_sidebars/version-${version}-sidebars.json"
	-rm  -r ./docs/versioned_docs/version-${version}
	jq 'del(.[] | select(. == "${version}"))' ./docs/versions.json > tmp.json && mv tmp.json ./docs/versions.json
	grep -v '"${version}": {label: "${version}", banner: "none", path: "${version}"},' ./docs/docusaurus.config.ts  > tmp.config.ts && mv tmp.config.ts ./docs/docusaurus.config.ts

.PHONY: ui ui-user build all clean dev dev-open lint lint-admin lint-api no-changes fmt tidy gen-docs-release deprecate-docs-release remove-docs-version
