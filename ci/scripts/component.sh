#!/bin/bash -eux

pushd dis-web-mount-check
  make test-component
popd
