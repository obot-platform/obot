# Makefile for Go project

default: build

# All target
all:
	$(MAKE) ui
	$(MAKE) build

ui:
	cd ui/admin && \
	npm install

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
	npm run format && \
	npm run lint

package-tools:
	./tools/package-tools.sh

in-docker-build: all
	$(MAKE) package-tools

no-changes:
	@if [ -n "$$(git status --porcelain)" ]; then \
		git status --porcelain; \
		git --no-pager diff; \
		echo "Encountered dirty repo!"; \
		exit 1; \
	fi

.PHONY: ui build all clean dev lint lint-admin lint-api no-changes fmt tidy
