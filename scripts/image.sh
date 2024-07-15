#!/bin/bash

set -euo pipefail

cd $(dirname $0)/../

source scripts/version.sh

BUILDER='cnrancher'
TARGET_PLATFORMS='linux/amd64,linux/arm64'

docker buildx ls | grep ${BUILDER} || \
    docker buildx create --name=${BUILDER} --platform=${TARGET_PLATFORMS}

set -x

IMAGE="${REPO}/rancher-flat-network-operator:${TAG}"
docker buildx build -f package/operator/Dockerfile \
    --builder ${BUILDER} --build-arg VERSION=${TAG} \
    -t "$IMAGE" \
    --platform=${TARGET_PLATFORMS} .
echo "Build $IMAGE"

IMAGE="${REPO}/rancher-flat-network-deploy:${TAG}"
docker buildx build -f package/deploy/Dockerfile \
    --builder ${BUILDER} --build-arg VERSION=${TAG} \
    -t "$IMAGE" \
    --platform=${TARGET_PLATFORMS} .
echo "Build $IMAGE"

IMAGE="${REPO}/rancher-flat-network-cni:${TAG}"
docker buildx build -f package/cni/Dockerfile \
    --builder ${BUILDER} --build-arg VERSION=${TAG} \
    -t "$IMAGE" \
    --platform=${TARGET_PLATFORMS} .
echo "Build $IMAGE"
