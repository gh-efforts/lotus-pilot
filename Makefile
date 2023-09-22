 SHELL=/usr/bin/env bash

 all: build
.PHONY: all

unexport GOFLAGS

ldflags=-X=github.com/gh-efforts/lotus-pilot/build.CurrentCommit=+git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))
ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags="$(ldflags)"

build:
	rm -f lotus-pilot
	go build $(GOFLAGS) -o lotus-pilot ./cmd/lotus-pilot
.PHONY: build

clean:
	rm -f lotus-pilot
	go clean