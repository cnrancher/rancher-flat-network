#!/bin/bash

set -euo pipefail

cd $(dirname $0)/../

REPO=${REPO:-'cnrancher'}
TAG=${TAG:-'latest'}
BUILDER='rancher-flat-network'
TARGET_PLATFORMS='linux/arm64,linux/amd64'

LOGLEVEL_VERSION=${LOGLEVEL_VERSION:-'v0.1.6'}
STATIC_IPAM_VERSION=${STATIC_IPAM_VERSION:-'v1.5.1'}
KUBECTL_VERSION=${KUBECTL_VERSION:-'v1.30.2'}

BUILDX_OPTIONS=${BUILDX_OPTIONS:-''} # Set to '--push' to upload images

docker buildx ls | grep ${BUILDER} || \
    docker buildx create --name=${BUILDER} --platform=${TARGET_PLATFORMS}

echo "Start build images"
set -x

docker buildx build -f package/operator/Dockerfile \
    --builder ${BUILDER} --build-arg LOGLEVEL_VERSION=${LOGLEVEL_VERSION} \
    -t "${REPO}/rancher-flat-network-operator:${TAG}" \
    --platform=${TARGET_PLATFORMS} ${BUILDX_OPTIONS} .

docker buildx build -f package/cni/Dockerfile \
    --builder ${BUILDER} --build-arg STATIC_IPAM_VERSION=${STATIC_IPAM_VERSION} \
    -t "${REPO}/rancher-flat-network-cni:${TAG}" \
    --platform=${TARGET_PLATFORMS} ${BUILDX_OPTIONS} .

docker buildx build -f package/deploy/Dockerfile \
    --builder ${BUILDER} --build-arg KUBECTL_VERSION=${KUBECTL_VERSION} \
    -t "${REPO}/rancher-flat-network-deploy:${TAG}" \
    --platform=${TARGET_PLATFORMS} ${BUILDX_OPTIONS} .

echo "Image: Done"
