# Makefile for Go project

default: build

# All target
all:
	$(MAKE) ui
	$(MAKE) build

ui:
	cd ui/admin && \
	npm install && \
    touch build/client/placeholder && \
	touch build/client/assets/_placeholder

touch:
	mkdir -p ui/admin/build/client/assets && \
    touch ui/admin/build/client/placeholder && \
	touch ui/admin/build/client/assets/_placeholder

clean:
	rm -rf ui/admin/build
	$(MAKE) touch

# Build the project
build: touch
	go build -o bin/otto -v

.PHONY: ui build all touch clean
