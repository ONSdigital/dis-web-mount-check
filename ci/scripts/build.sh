#!/bin/bash -eux

pushd dis-web-mount-check
  make build
  cp build/dis-web-mount-check Dockerfile.concourse ../build
popd
