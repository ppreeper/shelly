SHELL:=/usr/bin/env sh
UID:=$(shell id -u)
GID:=$(shell id -g)

.PHONY: bashly
bashly:
	docker run --rm -it --user "$(UID)":"$(GID)" -e BASHLY_TAB_INDENT=1 -v "$(CURDIR):/app" docker.io/dannyben/bashly generate -u