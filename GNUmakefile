default: testacc

# generate API client
OAPI_CODEGEN_VERSION=v2.0.0
PROJECT_DIR=$(shell pwd)
OAPI_CODEGEN_BIN=$(PROJECT_DIR)/bin/oapi-codegen
APIGEN_DIR=$(PROJECT_DIR)/pkg/cloudsdk/apigen

install-oapi-codegen:
	DIR=$(PROJECT_DIR)/bin VERSION=${OAPI_CODEGEN_VERSION} ./scripts/install-oapi-codegen.sh

prune-spec:
	@rm -f $(APIGEN_DIR)/**/*_gen.go

gen-spec: install-oapi-codegen prune-spec
	$(OAPI_CODEGEN_BIN) -generate types,client -o $(APIGEN_DIR)/mgmt/spec_gen.go -package apigen $(PROJECT_DIR)/risingwave-cloud-openapi/v1/mgmt.yaml
	$(OAPI_CODEGEN_BIN) -generate types,client -o $(APIGEN_DIR)/acc/spec_gen.go -package apigen $(PROJECT_DIR)/risingwave-cloud-openapi/v1/acc.yaml

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

ut:
	COLOR=ALWAYS go test -race -covermode=atomic -coverprofile=coverage.out -tags ut ./...
	@go tool cover -html coverage.out -o coverage.html
	@go tool cover -func coverage.out | fgrep total | awk '{print "Coverage:", $$3}'
