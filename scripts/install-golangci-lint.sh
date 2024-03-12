#!/bin/bash
# This script is used in Makefile.

echo "checking $VERSION for $DIR/golangci-lint"

$DIR/golangci-lint --version | grep $VERSION

if [ $? -eq 0 ]; then
    exit 0
fi

echo "installing $VERSION for $DIR/golangci-lint"

GOBIN=$DIR go install github.com/golangci/golangci-lint/cmd/golangci-lint@$VERSION
