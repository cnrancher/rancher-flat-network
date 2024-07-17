#!/usr/bin/env bash

set -euo pipefail

cd $(dirname $0)/../

type helm > /dev/null

TAG=${TAG:-'latest'}

# Cleanup
rm -rf build/charts &> /dev/null || true
mkdir -p build dist/artifacts &> /dev/null || true
cp -rf charts build/ &> /dev/null || true

# Update version
sed -i \
    -e 's/^version:.*/version: '${TAG/v/}'/' \
    -e 's/appVersion:.*/appVersion: '${TAG/v/}'/' \
    build/charts/rancher-flat-network/Chart.yaml

sed -i \
    -e 's/tag: v0.0.0/tag: "'${TAG}'"/' \
    build/charts/rancher-flat-network/values.yaml

helm package -d ./dist/artifacts ./build/charts/rancher-flat-network
