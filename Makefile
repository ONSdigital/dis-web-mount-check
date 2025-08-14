BINPATH ?= build

GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
WHITE  := $(shell tput -Txterm setaf 7)
CYAN   := $(shell tput -Txterm setaf 6)
RESET  := $(shell tput -Txterm sgr0)

APP=dis-web-mount-check
BUILD=build
BUILD_ARCH=$(BUILD)/$(GOOS)-$(GOARCH)
BUILD_AMD64=$(BUILD)/linux-amd64
BIN_DIR?=.

export GOOS?=$(shell go env GOOS)
export GOARCH?=$(shell go env GOARCH)

BUILD_TIME=$(shell date +%s)
GIT_COMMIT=$(shell git rev-parse HEAD)
VERSION ?= $(shell git tag --points-at HEAD | grep ^v | head -n 1)
# -w -s flags : omit DWARF debug, omit the symbol table
LDFLAGS = -ldflags "-w -s -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT) -X main.Version=$(VERSION)"

.PHONY: all
all: delimiter-AUDIT audit delimiter-LINTERS lint delimiter-UNIT-TESTS test delimiter-FINISH ## Runs multiple targets, audit, lint and test

.PHONY: audit
audit: ## Runs checks for security vulnerabilities on dependencies (including transient ones)
	go list -json -m all | nancy sleuth --exclude-vulnerability-file ./.nancy-ignore

.PHONY: build
build: ## Builds binary of application code and stores in bin directory as dis-web-mount-check
	go build -tags 'production' $(LDFLAGS) -o $(BINPATH)/$(APP)

.PHONY: buildamd64
buildamd64: ## Build app for testing on sandbox web 3
	@mkdir -p $(BUILD_AMD64)/$(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_AMD64)/$(BIN_DIR)/$(APP) main.go

.PHONY: pushamd64
pushamd64: ## Puts app built for sandbox web 3 onto sandbox web 3
	dp ssh sandbox web 3 -- mkdir -p test
	dp scp sandbox web 3 $(BUILD_AMD64)/$(BIN_DIR)/$(APP) test/$(APP)

.PHONY: convey
convey: ## Runs unit test suite and outputs results on http://127.0.0.1:8080/
	goconvey ./...

.PHONY: debug
debug: build ## Used to run code locally in debug mode
	HUMAN_LOG=1 go run $(LDFLAGS) -race main.go

.PHONY: delimiter-%
delimiter-%:
	@echo '===================${GREEN} $* ${RESET}==================='

.PHONY: fmt
fmt: ## Run Go formatting on code
	go fmt ./...

.PHONY: lint
lint: ## Used in ci to run linters against Go code
	golangci-lint run ./...

.PHONY: lint-local
lint-local: ## Use locally to run linters against Go code
	golangci-lint run ./...

.PHONY: test
test: ## Runs unit tests including checks for race conditions and returns coverage
	go test -race -cover ./...

.PHONY: cover
cover: ## run test coverage and open vscode to show results (within vscode, also do: COMMAND+SHIFT+P then: "Go: Toggle Test Coverage in Current Package")
	@echo '(within vscode, also do: COMMAND+SHIFT+P then: "Go: Toggle Test Coverage in Current Package")'
	go test ./... -coverprofile=coverage.out && code -r .

.PHONY: test-component
test-component: ## Runs component test suite
	exit

.PHONY: generate
generate: ## generate mocks_test.go
	go generate -v ./...

.PHONY: clean
clean: ## Clean up build directory
	rm -r $(BUILD)

.PHONY: help
help: ## Show help page for list of make targets
	@echo ''
	@echo 'Usage:'
	@echo '  ${YELLOW}make${RESET} ${GREEN}<target>${RESET}'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} { \
		if (/^[a-zA-Z_-]+:.*?##.*$$/) {printf "    ${YELLOW}%-20s${GREEN}%s${RESET}\n", $$1, $$2} \
		else if (/^## .*$$/) {printf "  ${CYAN}%s${RESET}\n", substr($$1,4)} \
		}' $(MAKEFILE_LIST)
