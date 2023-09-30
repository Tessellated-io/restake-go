#!/usr/bin/make -f

DOCKER := $(shell which docker)

CURRENT_DIR = $(shell pwd)
BUILDDIR ?= $(CURDIR)/build

BUILD_FLAGS := -tags "$(build_tags)" -ldflags '$(ldflags)'
# check for nostrip option
ifeq (,$(findstring nostrip,$(COSMOS_BUILD_OPTIONS)))
  BUILD_FLAGS += -trimpath
endif

# Check for debug option
ifeq (debug,$(findstring debug,$(COSMOS_BUILD_OPTIONS)))
  BUILD_FLAGS += -gcflags "all=-N -l"
endif

###############################################################################
###                                Protobuf                                 ###
###############################################################################

protoImageName=proto-genc
protoImage=$(DOCKER) run --rm -v $(CURDIR):/workspace --workdir /workspace/proto $(protoImageName)

proto-all: proto-build-docker proto-format proto-lint make proto-format proto-update-deps proto-gen

proto-build-docker:
	@echo "Building Docker Container '$(protoImageName)' for Protobuf Compilation"
	@docker build -t $(protoImageName) -f ./proto/Dockerfile .

proto-gen:
	@echo "Generating Protobuf Files"
	@$(protoImage) sh -c "cd .. && sh ./scripts/protocgen.sh" 

proto-format:
	@echo "Formatting Protobuf Files with Clang"
	@$(protoImage) find ./ -name "*.proto" -exec clang-format -i {} \;

proto-lint:
	@echo "Linting Protobuf Files With Buf"
	@$(protoImage) buf lint 

proto-check-breaking:
	@$(protoImage) buf breaking --against $(HTTPS_GIT)#branch=main

proto-update-deps:
	@echo "Updating Protobuf dependencies"	
	@$(protoImage) buf mod update

.PHONY: proto-all proto-gen proto-format proto-lint proto-check-breaking proto-update-deps

###############################################################################
###                                  Build                                  ###
###############################################################################

BUILD_TARGETS := build

build: BUILD_ARGS=

build-linux-amd64:
	@GOOS=linux GOARCH=amd64 LEDGER_ENABLED=false $(MAKE) build

build-linux-arm64:
	@GOOS=linux GOARCH=arm64 LEDGER_ENABLED=false $(MAKE) build

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	@cd ${CURRENT_DIR} && go $@ -mod=readonly $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	@mkdir -p $(BUILDDIR)/

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