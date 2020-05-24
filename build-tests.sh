#!/usr/bin/env bash
pushd test
docker build \
-t lizardfs-volume-plugin_test .
popd
