#!/usr/bin/env bash

set -x
make clean all

# This is hacky. Find a better way to integrate this in a CI
cp -f build/frconfig ~/tmp/fr/forgeops/docker/amster
cp -f build/frconfig ~/tmp/fr/forgeops/docker/git

#
#cd ui
#pub build
#cp -r build/web ~/tmp/fr/forgeops/docker/git/ui