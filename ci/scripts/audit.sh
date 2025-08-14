#!/bin/bash -eux

export cwd=$(pwd)

pushd $cwd/dis-web-mount-check
  make audit
popd
