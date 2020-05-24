#!/usr/bin/env bash
docker run -it --rm --privileged \
-v $(pwd)/plugin:/plugin \
lizardfs-volume-plugin_test $@
