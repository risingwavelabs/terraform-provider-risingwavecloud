PROJECT_DIR=$(shell pwd)

############################################################
### Go Lint 
############################################################
GOLANGCI_LINT_VERSION=v1.56.1
install-golangci-lint:
	DIR=$(PROJECT_DIR)/bin VERSION=${GOLANGCI_LINT_VERSION} ./scripts/install-golangci-lint.sh

lint-check: install-golangci-lint
	$(PROJECT_DIR)/bin/golangci-lint run --config $(PROJECT_DIR)/.golangci.yml

lint-fix: install-golangci-lint
	$(PROJECT_DIR)/bin/golangci-lint run --fix --config $(PROJECT_DIR)/.golangci.yml



############################################################
### OpenAPI Client Codegen 
############################################################
OAPI_CODEGEN_VERSION=v2.0.0
MOCKGEN_VERSION=v1.6.0
OAPI_CODEGEN_BIN=$(PROJECT_DIR)/bin/oapi-codegen
APIGEN_DIR=$(PROJECT_DIR)/internal/cloudsdk/apigen

install-oapi-codegen:
	DIR=$(PROJECT_DIR)/bin VERSION=${OAPI_CODEGEN_VERSION} ./scripts/install-oapi-codegen.sh

prune-spec:
	@rm -f $(APIGEN_DIR)/**/*_gen.go

gen-spec: install-oapi-codegen prune-spec
	$(OAPI_CODEGEN_BIN) -generate types,client -o $(APIGEN_DIR)/mgmt/spec_gen.go -package apigen $(PROJECT_DIR)/risingwave-cloud-openapi/v1/mgmt.yaml
	$(OAPI_CODEGEN_BIN) -generate types,client -o $(APIGEN_DIR)/acc/spec_gen.go -package apigen $(PROJECT_DIR)/risingwave-cloud-openapi/v1/acc.yaml



############################################################
### OpenAPI Client Codegen
############################################################
install-mockgen:
	DIR=$(PROJECT_DIR)/bin VERSION=${MOCKGEN_VERSION} ./scripts/install-mockgen.sh

gen-mock: install-mockgen
	@echo "no mock packages needed"

clean-gen-mock:
	@echo "no mock packages"


############################################################
### Tests
############################################################
testacc:
	TF_ACC=1 TF_LOG=INFO go test -v -timeout 30m github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/provider/acc

mockacc:
	RWC_ENDPOINT=testendpoint RWC_API_KEY=testkey RWC_API_SECRET=testsecret RWC_MOCK=1 TF_ACC=1 TF_LOG=INFO go test -v -timeout 30m github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/provider/acc

ut:
	COLOR=ALWAYS go test -race -covermode=atomic -coverprofile=coverage.out -tags ut ./...
	@go tool cover -html coverage.out -o coverage.html
	@go tool cover -func coverage.out | fgrep total | awk '{print "Coverage:", $$3}'



############################################################
### Common
############################################################

codegen: gen-mock gen-spec
