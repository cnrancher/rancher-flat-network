#!/usr/bin/env bash

set -euo pipefail

source $(dirname $0)/version.sh
cd $(dirname $0)/..
WORKINGDIR=$(pwd)

mkdir -p dist/artifacts
cp bin/flat-network-operator dist/artifacts/flat-network-operator${SUFFIX}

./scripts/package-helm.sh
