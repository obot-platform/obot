# Makefile for Go project

default: build

# All target
all:
	$(MAKE) ui
	$(MAKE) build

ui:
	cd ui/admin && \
	pnpm install

ui-user:
	cd ui/user && \
	pnpm install && \
	pnpm run build

clean:
	rm -rf ui/admin/build
	rm -rf ui/user/build

# Build the project
build:
	go build -o bin/otto8 -v

dev: ui
	./tools/dev.sh

# Lint the project
lint: lint-admin

lint-admin:
	cd ui/admin && \
	pnpm run format && \
	pnpm run lint

package-tools:
	./tools/package-tools.sh

in-docker-build:
	$(MAKE) ui-user
	$(MAKE) all
	$(MAKE) package-tools

no-changes:
	@if [ -n "$$(git status --porcelain)" ]; then \
		git status --porcelain; \
		git --no-pager diff; \
		echo "Encountered dirty repo!"; \
		exit 1; \
	fi

.PHONY: ui build all clean dev lint lint-admin lint-api no-changes fmt tidy
