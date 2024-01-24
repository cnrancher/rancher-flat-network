#!/usr/bin/env bash

set -euo pipefail

source $(dirname $0)/version.sh
cd $(dirname $0)/..
WORKINGDIR=$(pwd)

mkdir -p dist/artifacts
cp bin/macvlan-operator dist/artifacts/macvlan-operator${SUFFIX}

./scripts/package-helm.sh
