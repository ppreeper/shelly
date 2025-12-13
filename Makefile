SHELL:=/usr/bin/env sh
UID:=$(shell id -u)
GID:=$(shell id -g)

.PHONY: bashly
bashly:
	$(MAKE) -C bashlygen bashly

build-shelly:
	go build -o ./tmp/shelly ./cmd/shelly

.PHONY: shelly
shelly: build-shelly
	$(MAKE) -C shellygen shelly

.PHONY: build
build: bashly shelly