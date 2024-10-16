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
	go build -o bin/otto -v

dev: ui
	@echo "Starting dev otto server and admin UI..."
	./dev.sh

.PHONY: ui build all clean dev
