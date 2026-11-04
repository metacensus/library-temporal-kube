.PHONY: help install-tools-gen install-tools-lint proto mocks gen lint-proto-check fmt vet test test-race test-integration lint vuln tidy-check check release latest

.DEFAULT_GOAL := help

# Read out of go.mod so no copy can drift from what CI's setup-go uses.
GEN_TOOLCHAIN := go$(shell sed -n 's/^go //p' go.mod | head -1)
ifeq ($(GEN_TOOLCHAIN),go)
$(error could not read the Go version from go.mod; refusing to run unpinned)
endif
TOOL_PATH := PATH="$(CURDIR)/.bin:$$PATH"

GOLANGCI_LINT_VERSION := v2.14.0
TWF_VERSION := 0.16.0
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.8.0

help:
	@awk -F' — ' '/^## /{ sub(/^## /, ""); printf "  make %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## install-tools-gen — install buf, protoc plugins and mockery from tools/go.mod into .bin
install-tools-gen:
	@mkdir -p .bin
	@cd tools && GOWORK=off GOTOOLCHAIN=$(GEN_TOOLCHAIN) GOBIN=$(CURDIR)/.bin go install tool

## install-tools-lint — install golangci-lint into .bin
install-tools-lint: install-tools-gen
	@test -x $(CURDIR)/.bin/golangci-lint || \
		GOTOOLCHAIN=$(GEN_TOOLCHAIN) GOBIN=$(CURDIR)/.bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

## proto — regenerate protobuf code
proto: install-tools-gen
	@$(TOOL_PATH) buf generate

## mocks — regenerate mocks
mocks: install-tools-gen
	@GOWORK=off GOTOOLCHAIN=$(GEN_TOOLCHAIN) $(TOOL_PATH) mockery

## gen — proto + mocks
gen: proto mocks

## lint-proto-check — buf lint and format check
lint-proto-check: install-tools-gen
	@$(TOOL_PATH) buf lint
	@$(TOOL_PATH) buf format -d --exit-code

## fmt — rewrite with gofmt (skip gen/)
fmt:
	@gofmt -w $$(git ls-files '*.go' | grep -v '^gen/')

## vet — go vet, integration-tagged code included
vet:
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) go vet -tags=integration ./...

## test — unit tests
test:
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) go test ./...

## test-race — unit tests under the race detector
test-race:
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) go test -race -count=1 ./...

## test-integration — tests that start a cluster in a container
test-integration:
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) go test -tags=integration -count=1 ./...

## lint — proto lint, fmt check, vet, golangci-lint
lint: lint-proto-check install-tools-lint vet
	@test -z "$$(gofmt -l $$(git ls-files '*.go' | grep -v '^gen/') | tee /dev/stderr)"
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) $(TOOL_PATH) golangci-lint run ./...

## vuln — govulncheck over the module
vuln:
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) go run $(GOVULNCHECK) ./...

## tidy-check — go mod tidy leaves both modules unchanged
tidy-check:
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) go mod tidy
	@cd tools && GOTOOLCHAIN=$(GEN_TOOLCHAIN) go mod tidy
	@git diff --exit-code -- go.mod go.sum tools/go.mod tools/go.sum || (echo 'go.mod dirty; run go mod tidy' && exit 1)

## check — everything CI gates: lint, generated code fresh, twf, tidy, tests
check: lint
	@$(TOOL_PATH) buf generate
	@git diff --exit-code -- gen proto || (echo 'proto gen dirty; run make proto' && exit 1)
	@GOWORK=off GOTOOLCHAIN=$(GEN_TOOLCHAIN) $(TOOL_PATH) mockery
	@git diff --exit-code -- mocks || (echo 'mocks dirty; run make mocks' && exit 1)
	@npx -y @temporal-architect/twf@$(TWF_VERSION) check twf
	@$(MAKE) tidy-check
	@GOTOOLCHAIN=$(GEN_TOOLCHAIN) go test -race -count=1 ./...

## release — tag and push VERSION=x.y.z or the next TYPE=major|minor|patch; prompts unless YES=1
release:
	@set -e; \
	git fetch -q --tags origin main; \
	VERSION=$$(./scripts/version.sh "$(VERSION)" "$(TYPE)"); \
	TAG="v$$VERSION"; \
	if git ls-remote --exit-code --tags origin "refs/tags/$$TAG" >/dev/null 2>&1; then \
		echo "Error: Tag $$TAG already exists on origin"; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty; commit or clean it before releasing"; \
		exit 1; \
	fi; \
	if ! git merge-base --is-ancestor HEAD origin/main; then \
		echo "Error: HEAD is not on origin/main; refusing to tag"; \
		exit 1; \
	fi; \
	echo "About to tag and push $$TAG, which consumers pin in go.mod."; \
	if [ "$(YES)" != "1" ]; then \
		if [ -t 0 ]; then \
			printf "Proceed? [y/N] "; read -r reply; \
			case "$$reply" in y|Y|yes|YES) ;; *) echo "Aborted."; exit 1;; esac; \
		else \
			echo "Refusing to release non-interactively; pass YES=1 if you mean it."; \
			exit 1; \
		fi; \
	fi; \
	git tag -a "$$TAG" -m "Release $$VERSION" && \
	git push origin "$$TAG" && \
	echo "Released: $$TAG"

## latest — print the most recent version tag
latest:
	@bash -c '. scripts/version.sh && v=$$(get_latest_version) && [ -n "$$v" ] && echo v$$v || true'
