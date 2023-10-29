default: testacc

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

ut:
	COLOR=ALWAYS go test -race -covermode=atomic -coverprofile=coverage.out -tags ut ./... && go tool cover -html=./coverage.out -o coverage.html 
