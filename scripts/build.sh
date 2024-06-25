#!/usr/bin/env bash

set -euo pipefail

cd $(dirname $0)/../
WORKINGDIR=$(pwd)

source ./scripts/version.sh
echo "Version: ${VERSION}"

BUILD_FLAG=""
if [[ -z "${DEBUG:-}" ]]; then
    BUILD_FLAG="-extldflags -static -s -w"
fi
if [[ "${COMMIT}" != "UNKNOW" ]]; then
    BUILD_FLAG="${BUILD_FLAG} -X 'github.com/cnrancher/rancher-flat-network-operator/pkg/utils.GitCommit=${COMMIT}'"
fi
BUILD_FLAG="${BUILD_FLAG} -X 'github.com/cnrancher/rancher-flat-network-operator/pkg/utils.Version=${VERSION}'"

mkdir -p bin && cd bin
echo "Build rancher-flat-network-operator..."
CGO_ENABLED=0 go build -ldflags "${BUILD_FLAG}" -o rancher-flat-network-operator ..
echo "Build rancher-flat-network-cni..."
CGO_ENABLED=0 go build -ldflags "${BUILD_FLAG}" -o rancher-flat-network-cni ../cni/

echo "--------------------"
ls -alh rancher-flat-network-operator
ls -alh rancher-flat-network-cni
echo "--------------------"
