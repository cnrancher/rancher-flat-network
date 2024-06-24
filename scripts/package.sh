#!/usr/bin/env bash

set -euo pipefail

source $(dirname $0)/version.sh
cd $(dirname $0)/..
WORKINGDIR=$(pwd)

mkdir -p dist/artifacts
cp bin/rancher-flat-network-operator dist/artifacts/rancher-flat-network-operator${SUFFIX}

./scripts/package-helm.sh
