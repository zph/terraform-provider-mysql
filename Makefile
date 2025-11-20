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
	@echo '  make testversion8.0    Run tests against MySQL 8.0'
	@echo '  make testtidb8.5.3     Run tests against TiDB 8.5.3'
	@echo '  make acceptance        Run all acceptance tests'
	@echo '  make testcontainers-matrix  Run test matrix across all database versions'

default: help

build: fmtcheck ## Build the provider
	go install

test: testcontainers-matrix ## Run all acceptance tests
test-sequential: acceptance

# Run testcontainers tests with a matrix of all database versions
# Usage: make testcontainers-matrix TESTARGS="TestAccUser"
testcontainers-matrix: fmtcheck bin/terraform ## Run test matrix across all database versions
	@cd $(CURDIR) && PATH="$(CURDIR)/bin:${PATH}" PARALLEL=4 GOTOOLCHAIN=auto TF_ACC=1 go run scripts/test-runner.go $(if $(TESTARGS),$(TESTARGS),WithTestcontainers)

# Run testcontainers tests for a specific database image
# Usage: make testcontainers-image DOCKER_IMAGE=mysql:8.0
#        make testcontainers-image DOCKER_IMAGE=tidb:8.5.3
testcontainers-image: fmtcheck bin/terraform ## Run tests for a specific database image (set DOCKER_IMAGE)
	@PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers $(TEST) -v $(TESTARGS) -timeout=15m

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
testversion%: ## Run tests against MySQL version (e.g., testversion8.0)
	@DOCKER_IMAGE=mysql:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

testversion: ## Run tests against MySQL version (set MYSQL_VERSION)
	@DOCKER_IMAGE=mysql:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

# Percona test targets - use testcontainers
testpercona%: ## Run tests against Percona version (e.g., testpercona8.0)
	@DOCKER_IMAGE=percona:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

testpercona: ## Run tests against Percona version (set MYSQL_VERSION)
	@DOCKER_IMAGE=percona:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

testrdsdb%: ## Run tests against RDS MySQL version (requires MYSQL_ENDPOINT env vars)
	$(MAKE) MYSQL_VERSION=$* MYSQL_USERNAME=${MYSQL_USERNAME} MYSQL_HOST=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f1) MYSQL_PASSWORD=${MYSQL_PASSWORD} MYSQL_PORT=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f2) testrdsdb

testrdsdb: ## Run tests against Amazon RDS (requires MYSQL_ENDPOINT env vars)
	@echo 'Waiting for AMAZON RDS...'
	@while ! mysql -h "$(MYSQL_HOST)" -P "$(MYSQL_PORT)" -u "$(MYSQL_USERNAME)" -p"$(MYSQL_PASSWORD)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	$(MAKE) testacc

# TiDB test targets - use testcontainers
testtidb%: ## Run tests against TiDB version (e.g., testtidb8.5.3)
	@DOCKER_IMAGE=tidb:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

testtidb: ## Run tests against TiDB version (set MYSQL_VERSION)
	@DOCKER_IMAGE=tidb:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

# MariaDB test targets - use testcontainers
testmariadb%: ## Run tests against MariaDB version (e.g., testmariadb10.10)
	@DOCKER_IMAGE=mariadb:$* PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

testmariadb: ## Run tests against MariaDB version (set MYSQL_VERSION)
	@DOCKER_IMAGE=mariadb:$(MYSQL_VERSION) PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 GOTOOLCHAIN=auto go test -tags=testcontainers ./mysql/... -v $(if $(TESTARGS),-run "$(TESTARGS).*WithTestcontainers",-run WithTestcontainers) -timeout=30m

vet: ## Run go vet
	@echo "go vet ."
	@go vet $$(go list ./... | grep -v vendor/) ; if [ $$? -eq 1 ]; then \
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
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

errcheck: ## Run errcheck
	@sh -c "'$(CURDIR)/scripts/errcheck.sh'"

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

release: ## Create a release (tag, build, and optionally push to GitHub)
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

.PHONY: help build test testacc vet fmt fmtcheck errcheck vendor-status test-compile website website-test tag format-tag release
