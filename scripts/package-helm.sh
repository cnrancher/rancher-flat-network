#!/usr/bin/env bash

set -euxo pipefail

if ! hash helm 2>/dev/null; then
    exit 0
fi

cd $(dirname $0)/..
WORKINGDIR=$(pwd)
source ./scripts/version.sh

rm -rf build/charts &> /dev/null || true
mkdir -p build dist/artifacts &> /dev/null || true
cp -rf charts build/ &> /dev/null || true

sed -i \
    -e 's/^version:.*/version: '${HELM_VERSION}'/' \
    -e 's/appVersion:.*/appVersion: '${HELM_VERSION}'/' \
    build/charts/flat-network-operator/Chart.yaml

sed -i \
    -e 's/tag:.*/tag: '${HELM_TAG}'/' \
    build/charts/flat-network-operator/values.yaml

sed -i \
    -e 's/^version:.*/version: '${HELM_VERSION}'/' \
    -e 's/appVersion:.*/appVersion: '${HELM_VERSION}'/' \
    build/charts/flat-network-operator-crd/Chart.yaml

helm package -d ./dist/artifacts ./build/charts/flat-network-operator
helm package -d ./dist/artifacts ./build/charts/flat-network-operator-crd
