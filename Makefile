TEST?=$$(go list ./... |grep -v 'vendor')
GOFMT_FILES?=$$(find . -name '*.go' |grep -v vendor)
WEBSITE_REPO=github.com/hashicorp/terraform-website
PKG_NAME=mysql
# Last version before hashicorp relicensing to BSL
TERRAFORM_VERSION=1.5.6
TERRAFORM_OS=$(shell uname -s | tr A-Z a-z)
# Testcontainers-based testing - no need for manual Docker management
DATESTAMP=$(shell date "+%Y%m%d")
SHA_SHORT=$(shell git describe --match=FORCE_NEVER_MATCH --always --abbrev=40 --dirty --abbrev)
MOST_RECENT_UPSTREAM_TAG=$(shell git for-each-ref refs/tags --sort=-taggerdate --format="%(refname)" | head -1 | grep -E -o "v\d+\.\d+\.\d+")

# Set correct OS_ARCH on Mac
UNAME := $(shell uname -s)
HW := $(shell uname -m)
ifeq ($(HW),arm64)
	ARCH=$(HW)
else
	ARCH=amd64
endif

ifeq ($(UNAME),Darwin)
	OS_ARCH=darwin_$(ARCH)
else
	ARCH=amd64
	OS_ARCH=linux_$(ARCH)
endif

HOSTNAME=registry.terraform.io
NAMESPACE=zph
NAME=mysql
VERSION=9.9.9
## on linux base os
TERRAFORM_PLUGINS_DIRECTORY=~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ''
	@echo 'Examples:'
	@echo '  make build              Build the provider'
	@echo '  make release            Create a release PR branch (PR-based workflow)'
	@echo '  make testversion8.0    Run tests against MySQL 8.0'
	@echo '  make testtidb8.5.3     Run tests against TiDB 8.5.3'
	@echo '  make acceptance        Run all acceptance tests'
	@echo '  make testcontainers-matrix  Run test matrix across all database versions'

default: help

build: fmtcheck ## Build the provider
	go install

clean: ## Aggressively clear Docker cache and test artifacts
	@echo "Clearing Docker cache and test artifacts..."
	@# Remove testcontainers-related images (mysql, percona, mariadb, tidb)
	@docker images --format "{{.Repository}}:{{.Tag}}" | grep -E "(mysql|percona|mariadb|tidb|pingcap)" | xargs -r docker rmi -f 2>/dev/null || true
	@# Remove Docker manifests for problematic images (force remove even if they don't exist)
	@for img in mysql:5.6 mysql:5.7 percona:5.7 percona:8.0; do \
		docker manifest rm $$img 2>/dev/null || true; \
	done
	@# Prune build cache (all, not just 24h)
	@docker builder prune -af 2>/dev/null || true
	@# Prune unused images (all, not just 24h)
	@docker image prune -af 2>/dev/null || true
	@# Prune unused containers
	@docker container prune -f 2>/dev/null || true
	@# Prune unused networks (but keep default networks)
	@docker network prune -f 2>/dev/null || true
	@# Clear testcontainers temp files
	@rm -rf /tmp/testcontainers-* 2>/dev/null || true
	@# Clear Docker's content-addressable storage for problematic images (if possible)
	@echo "Docker cache cleared. Note: For MySQL 5.6/5.7 and Percona on Apple Silicon,"
	@echo "you may need to restart Docker Desktop to fully clear manifest cache."

build-tiup-playground-image: ## Pre-build TiUP Playground Docker image for caching
	@echo "Building TiUP Playground Docker image..."
	@if [ ! -f Dockerfile.tiup-playground ]; then \
		echo "ERROR: Dockerfile.tiup-playground not found"; \
		exit 1; \
	fi
	@docker build -f Dockerfile.tiup-playground -t terraform-provider-mysql-tiup-playground:latest .
	@echo "✓ TiUP Playground image built successfully: terraform-provider-mysql-tiup-playground:latest"

test: testcontainers-matrix ## Run all acceptance tests
test-sequential: acceptance

# Run testcontainers tests with a matrix of all database versions
# Usage: make testcontainers-matrix TESTARGS="TestAccUser"
testcontainers-matrix: fmtcheck bin/terraform ## Run test matrix across all database versions
	@cd $(CURDIR) && PATH="$(CURDIR)/bin:${PATH}" PARALLEL=4 TF_ACC=1 go run scripts/test-runner.go $(if $(TESTARGS),$(TESTARGS),WithTestcontainers)

# Run testcontainers tests for a specific database image
# Usage: make testcontainers-image DOCKER_IMAGE=mysql:8.0
#        make testcontainers-image DOCKER_IMAGE=tidb:8.5.3
testcontainers-image: fmtcheck bin/terraform ## Run tests for a specific database image (set DOCKER_IMAGE)
	@PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers $(TEST) -v $(TESTARGS) -timeout=15m

bin/terraform: ## Download Terraform binary
	mkdir -p "$(CURDIR)/bin"
	curl -sfL https://releases.hashicorp.com/terraform/$(TERRAFORM_VERSION)/terraform_$(TERRAFORM_VERSION)_$(TERRAFORM_OS)_$(ARCH).zip > $(CURDIR)/bin/terraform.zip
	(cd $(CURDIR)/bin/ ; unzip terraform.zip)

testacc: fmtcheck bin/terraform ## Run acceptance tests (requires MYSQL_ENDPOINT env vars)
	PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test $(TEST) -v $(TESTARGS) -timeout=90s

# TiDB versions: latest of each minor series (must match .github/workflows/main.yml TIDB_VERSIONS)
# 6.1.x → 6.1.7, 6.5.x → 6.5.12, 7.1.x → 7.1.6, 7.5.x → 7.5.7, 8.1.x → 8.1.2, 8.5.x → 8.5.3
acceptance: testversion5.6 testversion5.7 testversion8.0 testpercona5.7 testpercona8.0 testmariadb10.3 testmariadb10.8 testmariadb10.10 testtidb6.1.7 testtidb6.5.12 testtidb7.1.6 testtidb7.5.7 testtidb8.1.2 testtidb8.5.3 ## Run all acceptance tests across all database versions

# MySQL test targets - use testcontainers
# Preferred format: test-mysql-VERSION (e.g., test-mysql-5.6)
test-mysql-%: ## Run tests against MySQL version (e.g., test-mysql-8.0)
	@$(MAKE) testversion$*

testversion%: ## Run tests against MySQL version (e.g., testversion8.0) [backwards compatible]
	@# MySQL 5.6 and 5.7 don't have ARM64 builds - Docker Desktop on Apple Silicon has manifest cache issues
	@# The workaround: restart Docker Desktop or use CI (GitHub Actions uses linux/amd64)
	@if [ "$*" = "5.6" ] || [ "$*" = "5.7" ]; then \
		echo "WARNING: MySQL $* doesn't have ARM64 support. Docker Desktop manifest cache may cause issues."; \
		echo "If tests fail with 'no match for platform in manifest', try: make clean && restart Docker Desktop"; \
		docker rmi mysql:$* 2>/dev/null || true; \
		docker manifest rm mysql:$* 2>/dev/null || true; \
		docker pull --platform linux/amd64 mysql:$* 2>&1 | grep -v "no match" || true; \
		DOCKER_DEFAULT_PLATFORM=linux/amd64 DOCKER_IMAGE=mysql:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m; \
	else \
		DOCKER_IMAGE=mysql:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m; \
	fi

testversion: ## Run tests against MySQL version (set MYSQL_VERSION)
	@DOCKER_IMAGE=mysql:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m

# Percona test targets - use testcontainers
# Preferred format: test-percona-VERSION (e.g., test-percona-8.0)
test-percona-%: ## Run tests against Percona version (e.g., test-percona-8.0)
	@$(MAKE) testpercona$*

testpercona%: ## Run tests against Percona version (e.g., testpercona8.0) [backwards compatible]
	@# Percona 5.7 and 8.0 don't have ARM64 builds, so pre-pull with platform specification for Apple Silicon
	@if [ "$*" = "5.7" ] || [ "$*" = "8.0" ]; then \
		echo "Pre-pulling percona:$* with platform linux/amd64 for Apple Silicon compatibility..."; \
		docker rmi percona:$* 2>/dev/null || true; \
		docker manifest rm percona:$* 2>/dev/null || true; \
		docker pull --platform linux/amd64 percona:$* || true; \
	fi
	@DOCKER_IMAGE=percona:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m

testpercona: ## Run tests against Percona version (set MYSQL_VERSION)
	@DOCKER_IMAGE=percona:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m

testrdsdb%: ## Run tests against RDS MySQL version (requires MYSQL_ENDPOINT env vars)
	$(MAKE) MYSQL_VERSION=$* MYSQL_USERNAME=${MYSQL_USERNAME} MYSQL_HOST=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f1) MYSQL_PASSWORD=${MYSQL_PASSWORD} MYSQL_PORT=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f2) testrdsdb

testrdsdb: ## Run tests against Amazon RDS (requires MYSQL_ENDPOINT env vars)
	@echo 'Waiting for AMAZON RDS...'
	@while ! mysql -h "$(MYSQL_HOST)" -P "$(MYSQL_PORT)" -u "$(MYSQL_USERNAME)" -p"$(MYSQL_PASSWORD)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	$(MAKE) testacc

# TiDB test targets - use testcontainers
# Preferred format: test-tidb-VERSION (e.g., test-tidb-8.5.3)
test-tidb-%: ## Run tests against TiDB version (e.g., test-tidb-8.5.3)
	@$(MAKE) testtidb$*

testtidb%: ## Run tests against TiDB version (e.g., testtidb8.5.3) [backwards compatible]
	@DOCKER_IMAGE=tidb:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m

testtidb: ## Run tests against TiDB version (set MYSQL_VERSION)
	@DOCKER_IMAGE=tidb:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m

# MariaDB test targets - use testcontainers
# Preferred format: test-mariadb-VERSION (e.g., test-mariadb-10.10)
test-mariadb-%: ## Run tests against MariaDB version (e.g., test-mariadb-10.10)
	@$(MAKE) testmariadb$*

testmariadb%: ## Run tests against MariaDB version (e.g., testmariadb10.10) [backwards compatible]
	@DOCKER_IMAGE=mariadb:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m

testmariadb: ## Run tests against MariaDB version (set MYSQL_VERSION)
	@DOCKER_IMAGE=mariadb:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=30m

vet: ## Run go vet
	@echo "go vet ."
	@go vet $$(go list ./... | grep -v vendor/ | grep -v '/scripts') ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

fmt: ## Format Go code
	gofmt -w $(GOFMT_FILES)

deps: ## Update dependencies and vendor
	go mod tidy
	go mod vendor

fmtcheck: ## Check Go code formatting
	@echo "==> Checking that code complies with gofmt requirements..."
	@gofmt_files=$$(gofmt -l `find . -name '*.go' | grep -v vendor`); \
	if [ -n "$$gofmt_files" ]; then \
		echo 'gofmt needs running on the following files:'; \
		echo "$$gofmt_files"; \
		echo "You can use the command: \`make fmt\` to reformat code."; \
		exit 1; \
	fi

errcheck: ## Run errcheck
	@echo "==> Checking for unchecked errors..."
	@if ! which errcheck > /dev/null; then \
		echo "==> Installing errcheck..."; \
		go install github.com/kisielk/errcheck@latest; \
	fi
	@err_files=$$(errcheck -ignoretests \
		-ignore 'github.com/hashicorp/terraform/helper/schema:Set' \
		-ignore 'bytes:.*' \
		-ignore 'io:Close|Write' \
		$$(go list ./... | grep -v /vendor/)); \
	if [ -n "$$err_files" ]; then \
		echo 'Unchecked errors found in the following places:'; \
		echo "$$err_files"; \
		echo "Please handle returned errors. You can check directly with \`make errcheck\`"; \
		exit 1; \
	fi

lint: ## Run golangci-lint (correctness-focused linters)
	@echo "==> Running golangci-lint..."
	@GOPATH_BIN=$$(go env GOPATH)/bin; \
	GOLANGCI_LINT=$$GOPATH_BIN/golangci-lint; \
	if [ ! -f $$GOLANGCI_LINT ]; then \
		echo "==> Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$GOPATH_BIN latest; \
	fi; \
	$$GOLANGCI_LINT run ./mysql/... ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Linter found issues. Please review and fix them before submitting code."; \
		exit 1; \
	fi

vendor-status: ## Show vendor status
	@govendor status

test-compile: ## Compile tests without running them
	@if [ "$(TEST)" = "./..." ]; then \
		echo "ERROR: Set TEST to a specific package. For example,"; \
		echo "  make test-compile TEST=./$(PKG_NAME)"; \
		exit 1; \
	fi
	go test -c $(TEST) $(TESTARGS)

website: ## Generate website documentation
ifeq (,$(wildcard $(GOPATH)/src/$(WEBSITE_REPO)))
	echo "$(WEBSITE_REPO) not found in your GOPATH (necessary for layouts and assets), get-ting..."
	git clone https://$(WEBSITE_REPO) $(GOPATH)/src/$(WEBSITE_REPO)
endif
	( cd "$(GOPATH)/src/$(WEBSITE_REPO)" && git checkout 6d41be434cf85392bc9de773d8a5a8d571a195ad )

	@$(MAKE) -C $(GOPATH)/src/$(WEBSITE_REPO) website-provider PROVIDER_PATH=$(shell pwd) PROVIDER_NAME=$(PKG_NAME)

install: ## Install provider to Terraform plugins directory
	mkdir -p ${TERRAFORM_PLUGINS_DIRECTORY}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd examples && rm -rf .terraform
	cd examples && make init

re-install: ## Reinstall provider (removes lock file first)
	rm -f examples/.terraform.lock.hcl
	rm -f ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd examples && rm -rf .terraform
	cd examples && terraform init

format-tag: ## Format tag string
	@echo $(MOST_RECENT_UPSTREAM_TAG)-$(DATESTAMP)-$(SHA_SHORT)

tag: ## Create git tag from VERSION file
	@echo git tag -a $(shell cat VERSION) -m $(shell cat VERSION)
	@git tag -a v$(shell cat VERSION) -m v$(shell cat VERSION)

release-local: ## Create a release locally (for testing - use 'make release' for PR-based workflow)
	@VERSION=$$(cat VERSION); \
	TAG="v$$VERSION"; \
	echo "Checking if tag $$TAG already exists..."; \
	if git rev-parse "$$TAG" >/dev/null 2>&1; then \
		echo "Tag $$TAG already exists!"; \
		echo "Current version from VERSION file: $$VERSION"; \
		\
		LAST_TAG=$$(git tag --list "v*" | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+" | sort -V | tail -1); \
		if [ -n "$$LAST_TAG" ]; then \
			LAST_VERSION=$${LAST_TAG#v}; \
			echo "Most recent tag: $$LAST_TAG (version: $$LAST_VERSION)"; \
			\
			MAJOR_MINOR=$$(echo "$$LAST_VERSION" | sed -E 's/\.[0-9]+$$//'); \
			BUILD_NUM=$$(echo "$$LAST_VERSION" | sed -E 's/.*\.([0-9]+)$$/\1/'); \
			NEXT_BUILD=$$((BUILD_NUM + 1)); \
			NEXT_VERSION="$$MAJOR_MINOR.$$NEXT_BUILD"; \
			NEXT_TAG="v$$NEXT_VERSION"; \
			\
			echo ""; \
			echo "Suggested next tag: $$NEXT_TAG"; \
			echo -n "Enter next tag (or press Enter to use $$NEXT_TAG): "; \
			read USER_TAG; \
			if [ -z "$$USER_TAG" ]; then \
				USER_TAG="$$NEXT_TAG"; \
			fi; \
			if [ "$${USER_TAG#v}" = "$$USER_TAG" ]; then \
				TAG="v$$USER_TAG"; \
			else \
				TAG="$$USER_TAG"; \
			fi; \
			VERSION=$${TAG#v}; \
		else \
			echo "Could not determine next tag. Please enter manually:"; \
			read -p "Enter next tag: " USER_TAG; \
			if [ "$${USER_TAG#v}" = "$$USER_TAG" ]; then \
				TAG="v$$USER_TAG"; \
			else \
				TAG="$$USER_TAG"; \
			fi; \
			VERSION=$${TAG#v}; \
		fi; \
	else \
		echo "Tag $$TAG does not exist. Using version from VERSION file: $$VERSION"; \
	fi; \
	\
	ORIGINAL_VERSION=$$(cat VERSION); \
	if [ "$$VERSION" != "$$ORIGINAL_VERSION" ]; then \
		echo ""; \
		echo "Updating VERSION file from $$ORIGINAL_VERSION to $$VERSION..."; \
		echo "$$VERSION" > VERSION; \
		echo "VERSION file updated."; \
	fi; \
	\
	echo ""; \
	echo "========================================="; \
	echo "Release Summary:"; \
	echo "  Tag: $$TAG"; \
	echo "  Version: $$VERSION"; \
	echo "========================================="; \
	echo ""; \
	echo -n "Do you want to create tag $$TAG? (yes/no): "; \
	read CONFIRM_TAG; \
	if [ "$$CONFIRM_TAG" != "yes" ]; then \
		echo "Tag creation cancelled."; \
		exit 1; \
	fi; \
	\
	echo ""; \
	echo "Creating tag $$TAG..."; \
	if [ "$$VERSION" != "$$ORIGINAL_VERSION" ]; then \
		git add VERSION || exit 1; \
		git commit -m "Update VERSION to $$VERSION" || exit 1; \
		echo "VERSION file change committed."; \
	fi; \
	git tag -a "$$TAG" -m "$$TAG" || exit 1; \
	echo "Tag $$TAG created successfully."; \
	\
	echo ""; \
	echo -n "Do you want to deploy this as a GitHub release? (yes/no): "; \
	read CONFIRM_RELEASE; \
	if [ "$$CONFIRM_RELEASE" != "yes" ]; then \
		echo "GitHub release cancelled. Tag created but not released."; \
		echo "You can release it later with: goreleaser release --clean"; \
		exit 0; \
	fi; \
	\
	echo ""; \
	echo "Running goreleaser to create GitHub release..."; \
	goreleaser release --clean --verbose || exit 1; \
	\
	echo ""; \
	echo -n "Do you want to push tags and commits to GitHub? (yes/no): "; \
	read CONFIRM_PUSH; \
	if [ "$$CONFIRM_PUSH" != "yes" ]; then \
		echo "Push cancelled. Tag and release created locally."; \
		exit 0; \
	fi; \
	\
	echo ""; \
	echo "Pushing to GitHub..."; \
	git push origin --tags || exit 1; \
	git push origin HEAD || exit 1; \
	echo ""; \
	echo "Release complete! Tag $$TAG has been pushed to GitHub."

release: ## Create a release PR branch (tag, push branch and tag, then create PR to merge to default branch)
	@go run scripts/make-release.go

.PHONY: help build test testacc vet fmt fmtcheck errcheck lint vendor-status test-compile website website-test tag format-tag release release-local
