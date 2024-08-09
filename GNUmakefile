TEST?=$$(go list ./... |grep -v 'vendor')
GOFMT_FILES?=$$(find . -name '*.go' |grep -v vendor)
WEBSITE_REPO=github.com/hashicorp/terraform-website
PKG_NAME=mysql
# Last version before hashicorp relicensing to BSL
TERRAFORM_VERSION=1.5.6
TERRAFORM_OS=$(shell uname -s | tr A-Z a-z)
TEST_USER=root
TEST_PASSWORD=my-secret-pw
DATESTAMP=$(shell date "+%Y%m%d")
SHA_SHORT=$(shell git describe --match=FORCE_NEVER_MATCH --always --abbrev=40 --dirty --abbrev)
MOST_RECENT_UPSTREAM_TAG=$(shell git for-each-ref refs/tags --sort=-taggerdate --format="%(refname)" | head -1 | grep -E -o "v\d+\.\d+\.\d+")

OS_ARCH=linux_amd64
# Set correct OS_ARCH on Mac
UNAME := $(shell uname -s)
ifeq ($(UNAME),Darwin)
	HW := $(shell uname -m)
	ifeq ($(HW),arm64)
		ARCH=$(HW)
	else
		ARCH=amd64
	endif
	OS_ARCH=darwin_$(ARCH)
endif

HOSTNAME=registry.terraform.io
NAMESPACE=zph
NAME=mysql
VERSION=9.9.9
## on linux base os
TERRAFORM_PLUGINS_DIRECTORY=~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

default: build

build: fmtcheck
	go install

test: acceptance

bin/terraform:
	mkdir -p "$(CURDIR)/bin"
	curl -sfL https://releases.hashicorp.com/terraform/$(TERRAFORM_VERSION)/terraform_$(TERRAFORM_VERSION)_$(TERRAFORM_OS)_$(ARCH).zip > $(CURDIR)/bin/terraform.zip
	(cd $(CURDIR)/bin/ ; unzip terraform.zip)

testacc: fmtcheck bin/terraform
	PATH="$(CURDIR)/bin:${PATH}" TF_ACC=1 go test $(TEST) -v $(TESTARGS) -timeout=90s

acceptance: testversion5.6 testversion5.7 testversion8.0 testpercona5.7 testpercona8.0 testmariadb10.3 testmariadb10.8 testmariadb10.10 testtidb6.1.0 testtidb7.5.2

testversion%:
	$(MAKE) MYSQL_VERSION=$* MYSQL_PORT=33$(shell echo "$*" | tr -d '.') testversion

testversion:
	-docker run --rm --name test-mysql$(MYSQL_VERSION) -e MYSQL_ROOT_PASSWORD="$(TEST_PASSWORD)" -d -p $(MYSQL_PORT):3306 mysql:$(MYSQL_VERSION)
	@echo 'Waiting for MySQL...'
	@while ! mysql -h 127.0.0.1 -P $(MYSQL_PORT) -u "$(TEST_USER)" -p"$(TEST_PASSWORD)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	-mysql -h 127.0.0.1 -P $(MYSQL_PORT) -u "$(TEST_USER)" -p"$(TEST_PASSWORD)" -e "INSTALL PLUGIN mysql_no_login SONAME 'mysql_no_login.so';"
	MYSQL_USERNAME="$(TEST_USER)" MYSQL_PASSWORD="$(TEST_PASSWORD)" MYSQL_ENDPOINT=127.0.0.1:$(MYSQL_PORT) $(MAKE) testacc
	-docker rm -f test-mysql$(MYSQL_VERSION)

testpercona%:
	$(MAKE) MYSQL_VERSION=$* MYSQL_PORT=34$(shell echo "$*" | tr -d '.') testpercona

testpercona:
	-docker run --rm --name test-percona$(MYSQL_VERSION) -e MYSQL_ROOT_PASSWORD="$(TEST_PASSWORD)" -d -p $(MYSQL_PORT):3306 percona:$(MYSQL_VERSION)
	@echo 'Waiting for Percona...'
	@while ! mysql -h 127.0.0.1 -P $(MYSQL_PORT) -u "$(TEST_USER)" -p"$(TEST_PASSWORD)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	-mysql -h 127.0.0.1 -P $(MYSQL_PORT) -u "$(TEST_USER)" -p"$(TEST_PASSWORD)" -e "INSTALL PLUGIN mysql_no_login SONAME 'mysql_no_login.so';"
	MYSQL_USERNAME="$(TEST_USER)" MYSQL_PASSWORD="$(TEST_PASSWORD)" MYSQL_ENDPOINT=127.0.0.1:$(MYSQL_PORT) $(MAKE) testacc
	-docker rm -f test-percona$(MYSQL_VERSION)

testrdsdb%:
	$(MAKE) MYSQL_VERSION=$* MYSQL_USERNAME=${MYSQL_USERNAME} MYSQL_HOST=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f1) MYSQL_PASSWORD=${MYSQL_PASSWORD} MYSQL_PORT=$(shell echo ${MYSQL_ENDPOINT} | cut -d: -f2) testrdsdb

testrdsdb:
	@echo 'Waiting for AMAZON RDS...'
	@while ! mysql -h "$(MYSQL_HOST)" -P "$(MYSQL_PORT)" -u "$(MYSQL_USERNAME)" -p"$(MYSQL_PASSWORD)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	$(MAKE) testacc

testtidb%:
	$(MAKE) MYSQL_VERSION=$* MYSQL_PORT=34$(shell echo "$*" | tr -d '.') testtidb

# WARNING: this does not work as a bare task run, it only instantiates correctly inside the versioned TiDB task run
#          otherwise MYSQL_PORT and version are unset.
testtidb:
	@sh -c "'$(CURDIR)/scripts/tidb-test-cluster.sh' --init --port $(MYSQL_PORT) --version $(MYSQL_VERSION)"
	@echo 'Waiting for TiDB...'
	@while ! mysql -h 127.0.0.1 -P $(MYSQL_PORT) -u "$(TEST_USER)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	MYSQL_USERNAME="$(TEST_USER)" MYSQL_PASSWORD="" MYSQL_ENDPOINT=127.0.0.1:$(MYSQL_PORT) $(MAKE) testacc
	@sh -c "'$(CURDIR)/scripts/tidb-test-cluster.sh' --destroy"

testmariadb%:
	$(MAKE) MYSQL_VERSION=$* MYSQL_PORT=6$(shell echo "$*" | tr -d '.') testmariadb

testmariadb:
	-docker run --rm --name test-mariadb$(MYSQL_VERSION) -e MYSQL_ROOT_PASSWORD="$(TEST_PASSWORD)" -d -p $(MYSQL_PORT):3306 mariadb:$(MYSQL_VERSION)
	@echo 'Waiting for MySQL...'
	@while ! mysql -h 127.0.0.1 -P $(MYSQL_PORT) -u "$(TEST_USER)" -p"$(TEST_PASSWORD)" -e 'SELECT 1' >/dev/null 2>&1; do printf '.'; sleep 1; done ; echo ; echo "Connected!"
	MYSQL_USERNAME="$(TEST_USER)" MYSQL_PASSWORD="$(TEST_PASSWORD)" MYSQL_ENDPOINT=127.0.0.1:$(MYSQL_PORT) $(MAKE) testacc
	-docker rm -f test-mariadb$(MYSQL_VERSION)

vet:
	@echo "go vet ."
	@go vet $$(go list ./... | grep -v vendor/) ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

fmt:
	gofmt -w $(GOFMT_FILES)

deps:
	go mod tidy
	go mod vendor

fmtcheck:
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

errcheck:
	@sh -c "'$(CURDIR)/scripts/errcheck.sh'"

vendor-status:
	@govendor status

test-compile:
	@if [ "$(TEST)" = "./..." ]; then \
		echo "ERROR: Set TEST to a specific package. For example,"; \
		echo "  make test-compile TEST=./$(PKG_NAME)"; \
		exit 1; \
	fi
	go test -c $(TEST) $(TESTARGS)

website:
ifeq (,$(wildcard $(GOPATH)/src/$(WEBSITE_REPO)))
	echo "$(WEBSITE_REPO) not found in your GOPATH (necessary for layouts and assets), get-ting..."
	git clone https://$(WEBSITE_REPO) $(GOPATH)/src/$(WEBSITE_REPO)
endif
	( cd "$(GOPATH)/src/$(WEBSITE_REPO)" && git checkout 6d41be434cf85392bc9de773d8a5a8d571a195ad )

	@$(MAKE) -C $(GOPATH)/src/$(WEBSITE_REPO) website-provider PROVIDER_PATH=$(shell pwd) PROVIDER_NAME=$(PKG_NAME)

install:
	mkdir -p ${TERRAFORM_PLUGINS_DIRECTORY}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd examples && rm -rf .terraform
	cd examples && make init

re-install:
	rm -f examples/.terraform.lock.hcl
	rm -f ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd examples && rm -rf .terraform
	cd examples && terraform init

format-tag:
	@echo $(MOST_RECENT_UPSTREAM_TAG)-$(DATESTAMP)-$(SHA_SHORT)

tag:
	@echo git tag -a $(MOST_RECENT_UPSTREAM_TAG)-$(DATESTAMP)-$(SHA_SHORT) -m $(MOST_RECENT_UPSTREAM_TAG)-$(DATESTAMP)-$(SHA_SHORT)
	@git tag -a $(MOST_RECENT_UPSTREAM_TAG)-$(DATESTAMP)-$(SHA_SHORT) -m $(MOST_RECENT_UPSTREAM_TAG)-$(DATESTAMP)-$(SHA_SHORT)

release:
	@goreleaser release --clean --verbose
.PHONY: build test testacc vet fmt fmtcheck errcheck vendor-status test-compile website website-test tag format-tag
