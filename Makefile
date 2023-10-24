#!/usr/bin/make -f

DOCKER := $(shell which docker)

BUILDDIR ?= $(CURDIR)/build

BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git log -1 --format='%H')

DIRTY := -dirty
ifeq (,$(shell git status --porcelain))
	DIRTY := 
endif

VERSION := $(shell git describe --tags --exact-match 2>/dev/null)
# if VERSION is empty, then populate it with branch's name and raw commit hash
ifeq (,$(VERSION))
  VERSION := $(BRANCH)-$(COMMIT)
endif

VERSION := $(VERSION)$(DIRTY)

GIT_REVISION := $(shell git rev-parse HEAD)$(DIRTY)

GO_SYSTEM_VERSION = $(shell go version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f1-2)

ldflags= -X  github.com/tessellated-io/restake-go/cmd/restake-go/cmd.RestakeVersion=${VERSION} \
	-X  github.com/tessellated-io/restake-go/cmd/restake-go/cmd.GitRevision=${GIT_REVISION} \
	-X github.com/tessellated-io/restake-go/cmd/restake-go/cmd.GoVersion=${GO_SYSTEM_VERSION}

BUILD_FLAGS := -tags "$(build_tags)" -ldflags '$(ldflags)'

###############################################################################
###                                  Build                                  ###
###############################################################################

BUILD_TARGETS := build

build:
	mkdir -p $(BUILDDIR)/
	go build -mod=readonly -ldflags '$(ldflags)' -trimpath -o $(BUILDDIR) ./...;

install: go.sum
	go install $(BUILD_FLAGS) ./cmd/restake-go

clean:
	rm -rf $(BUILDDIR)/*

.PHONY: build

###############################################################################
###                          Tools & Dependencies                           ###
###############################################################################

go.sum: go.mod
	@go mod verify
	@go mod tidy


###############################################################################
###                                Linting                                  ###
###############################################################################

golangci_version=v1.53.3

lint-install:
	@echo "--> Installing golangci-lint $(golangci_version)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(golangci_version)

lint:
	@echo "--> Running linter"
	$(MAKE) lint-install
	@golangci-lint run  -c "./.golangci.yml"

lint-fix:
	@echo "--> Running linter"
	$(MAKE) lint-install
	@golangci-lint run  -c "./.golangci.yml" --fix


.PHONY: lint lint-fix