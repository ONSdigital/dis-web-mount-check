#!/bin/bash -eux

go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.12.2

pushd dis-web-mount-check
  make lint
popd
