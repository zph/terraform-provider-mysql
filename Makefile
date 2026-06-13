TEST?=./mysql/...
UNIT_TEST?=./internal/... ./mysql
UNIT_TEST_TIMEOUT?=5m
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
TESTCONTAINERS_TIMEOUT?=30m
TESTCONTAINERS_RUNNER=cd $(CURDIR) && PATH="$(CURDIR)/bin:${PATH}" PARALLEL="$${PARALLEL:-1}" go run scripts/test-runner.go --package "$(TEST)" --timeout "$(TESTCONTAINERS_TIMEOUT)" $(if $(VERBOSE),--verbose,)
MATRIX_POLICY_ARGS=--github-actions --fail-on-red $(if $(MATRIX_POLICY_SUMMARY_FILE),--summary-file "$(MATRIX_POLICY_SUMMARY_FILE)",)
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
	@echo '  make test-unit          Run unit tests without testcontainers'
	@echo '  make test-integration   Run testcontainers integration matrix'
	@echo '  make testcontainers-db DB=mysql VERSION=8.0'
	@echo '  make eol-versions      Check matrix patch drift and EOL warnings'
	@echo '  make eol-versions-ci   Enforce matrix version policy for CI'
	@echo '  make test VERBOSE=1    Run unit and integration tests, streaming integration output'
	@echo '  make acceptance        Run integration tests sequentially'
	@echo '  make testcontainers-matrix  Run test matrix across all database versions'

default: help

build: fmtcheck ## Build the provider
	go install

clean: ## Aggressively clear Docker cache and test artifacts
	@echo "Clearing Docker cache and test artifacts..."
	@# Remove testcontainers-related images (mysql, percona, mariadb, tidb)
	@docker images --format "{{.Repository}}:{{.Tag}}" | grep -E "(mysql|percona|mariadb|tidb|pingcap)" | xargs -r docker rmi -f 2>/dev/null || true
	@# Remove Docker manifests for problematic images (force remove even if they don't exist)
	@for img in mysql:5.7 percona:5.7 percona:8.0; do \
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
	@echo "Docker cache cleared. Note: For MySQL 5.7 and Percona on Apple Silicon,"
	@echo "you may need to restart Docker Desktop to fully clear manifest cache."

build-tiup-playground-image: ## Pre-build TiUP Playground Docker image for caching
	@echo "Building TiUP Playground Docker image..."
	@if [ ! -f Dockerfile.tiup-playground ]; then \
		echo "ERROR: Dockerfile.tiup-playground not found"; \
		exit 1; \
	fi
	@docker build -f Dockerfile.tiup-playground -t terraform-provider-mysql-tiup-playground:latest .
	@echo "✓ TiUP Playground image built successfully: terraform-provider-mysql-tiup-playground:latest"

test: test-unit test-integration ## Run unit tests, then integration tests
test-unit: fmtcheck ## Run unit tests that do not require testcontainers or external database servers
	@go test $(UNIT_TEST) $(if $(TESTARGS),-run "$(TESTARGS)",) -timeout=$(UNIT_TEST_TIMEOUT)

test-integration: testcontainers-matrix ## Run testcontainers integration tests
test-sequential: acceptance ## Run testcontainers integration tests sequentially

# Run testcontainers integration tests with a matrix of all database versions
# Usage: make testcontainers-matrix TESTARGS="TestAccUser"
testcontainers-matrix: fmtcheck bin/terraform ## Run integration test matrix across all database versions
	@PARALLEL=$(if $(VERBOSE),1,4); $(TESTCONTAINERS_RUNNER) $(TESTARGS)

# Run testcontainers tests for a specific database image
# Usage: make testcontainers-image DOCKER_IMAGE=mysql:8.0
#        make testcontainers-image DOCKER_IMAGE=tidb:8.5.5
testcontainers-image: fmtcheck bin/terraform ## Run tests for a specific database image (set DOCKER_IMAGE)
	@$(TESTCONTAINERS_RUNNER) --image "$(DOCKER_IMAGE)" $(TESTARGS)

testcontainers-db: fmtcheck bin/terraform ## Run tests for a database/version pair (set DB and VERSION)
	@if [ -z "$(DB)" ] || [ -z "$(VERSION)" ]; then \
		echo "ERROR: set DB and VERSION. Example: make testcontainers-db DB=mysql VERSION=8.0"; \
		exit 1; \
	fi
	@$(TESTCONTAINERS_RUNNER) --db "$(DB)" --version "$(VERSION)" $(TESTARGS)

eol-versions: ## Check matrix image patch drift and EOL warnings
	@go run scripts/update-test-matrix.go

eol-versions-ci: ## Enforce matrix version policy for CI
	@go run scripts/update-test-matrix.go $(MATRIX_POLICY_ARGS)

testcontainers-matrix-check: eol-versions-ci ## Enforce matrix version policy for CI

testcontainers-matrix-update: ## Update matrix image patch versions in known files
	@go run scripts/update-test-matrix.go --write

bin/terraform: ## Download Terraform binary
	mkdir -p "$(CURDIR)/bin"
	curl -sfL https://releases.hashicorp.com/terraform/$(TERRAFORM_VERSION)/terraform_$(TERRAFORM_VERSION)_$(TERRAFORM_OS)_$(ARCH).zip > $(CURDIR)/bin/terraform.zip
	(cd $(CURDIR)/bin/ ; unzip terraform.zip)

testacc: fmtcheck bin/terraform ## Run acceptance tests (requires MYSQL_ENDPOINT env vars)
	PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test $(TEST) -v $(TESTARGS) -timeout=90s

acceptance: fmtcheck bin/terraform ## Run integration test matrix sequentially
	@PARALLEL=1; $(TESTCONTAINERS_RUNNER) $(TESTARGS)

# MySQL test targets - use testcontainers
# Preferred format: test-mysql-VERSION (e.g., test-mysql-8.0)
test-mysql-%: ## Run tests against MySQL version (e.g., test-mysql-8.0)
	@$(MAKE) testcontainers-db DB=mysql VERSION="$*"

testversion%: ## Run tests against MySQL version (e.g., testversion8.0) [backwards compatible]
	@$(MAKE) testcontainers-db DB=mysql VERSION="$*"

testversion: ## Run tests against MySQL version (set MYSQL_VERSION)
	@$(MAKE) testcontainers-db DB=mysql VERSION="$(MYSQL_VERSION)"

# Percona test targets - use testcontainers
# Preferred format: test-percona-VERSION (e.g., test-percona-8.0)
test-percona-%: ## Run tests against Percona version (e.g., test-percona-8.0)
	@$(MAKE) testcontainers-db DB=percona VERSION="$*"

testpercona%: ## Run tests against Percona version (e.g., testpercona8.0) [backwards compatible]
	@$(MAKE) testcontainers-db DB=percona VERSION="$*"

testpercona: ## Run tests against Percona version (set MYSQL_VERSION)
	@$(MAKE) testcontainers-db DB=percona VERSION="$(MYSQL_VERSION)"

testrdsdb%: ## Run tests against RDS MySQL version (requires MYSQL_ENDPOINT env vars)
	$(MAKE) MYSQL_VERSION=$* MYSQL_USERNAME=${MYSQL_USERNAME} MYSQL_HOST=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f1) MYSQL_PASSWORD=${MYSQL_PASSWORD} MYSQL_PORT=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f2) testrdsdb

testrdsdb: ## Run tests against Amazon RDS (requires MYSQL_ENDPOINT env vars)
	@echo 'Waiting for AMAZON RDS...'
	@while ! mysql -h "$(MYSQL_HOST)" -P "$(MYSQL_PORT)" -u "$(MYSQL_USERNAME)" -p"$(MYSQL_PASSWORD)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	$(MAKE) testacc

# TiDB test targets - use testcontainers
# Preferred format: test-tidb-VERSION (e.g., test-tidb-8.5.5)
test-tidb-%: ## Run tests against TiDB version (e.g., test-tidb-8.5.5)
	@$(MAKE) testcontainers-db DB=tidb VERSION="$*"

testtidb%: ## Run tests against TiDB version (e.g., testtidb8.5.5) [backwards compatible]
	@$(MAKE) testcontainers-db DB=tidb VERSION="$*"

testtidb: ## Run tests against TiDB version (set MYSQL_VERSION)
	@$(MAKE) testcontainers-db DB=tidb VERSION="$(MYSQL_VERSION)"

# MariaDB test targets - use testcontainers
# Preferred format: test-mariadb-VERSION (e.g., test-mariadb-10.10)
test-mariadb-%: ## Run tests against MariaDB version (e.g., test-mariadb-10.10)
	@$(MAKE) testcontainers-db DB=mariadb VERSION="$*"

testmariadb%: ## Run tests against MariaDB version (e.g., testmariadb10.10) [backwards compatible]
	@$(MAKE) testcontainers-db DB=mariadb VERSION="$*"

testmariadb: ## Run tests against MariaDB version (set MYSQL_VERSION)
	@$(MAKE) testcontainers-db DB=mariadb VERSION="$(MYSQL_VERSION)"

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

.PHONY: help build test test-unit test-integration test-sequential testcontainers-matrix testcontainers-image testcontainers-db eol-versions eol-versions-ci testcontainers-matrix-check testcontainers-matrix-update testacc acceptance vet fmt fmtcheck errcheck vendor-status test-compile website website-test tag format-tag release release-local
