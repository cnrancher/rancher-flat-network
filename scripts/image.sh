#!/bin/bash

set -euxo pipefail

cd $(dirname $0)/../
WORKINGDIR=$(pwd)

# Build linux image
export GOOS=linux

# TODO: DEBUG
go mod tidy
./scripts/build.sh

docker build -t rancher-flat-network-operator -f ./package/operator/Dockerfile .
docker build -t rancher-flat-network-webhook-deploy -f ./package/webhook-deploy/Dockerfile .
docker build -t rancher-flat-network-cni -f ./package/cni/Dockerfile .

# exit 0

# TODO: DEBUG
docker tag  rancher-flat-network-operator harborlocal.hxstarrys.me/cnrancher/rancher-flat-network-operator:v0.0.0
docker push harborlocal.hxstarrys.me/cnrancher/rancher-flat-network-operator:v0.0.0

docker tag  rancher-flat-network-webhook-deploy harborlocal.hxstarrys.me/cnrancher/rancher-flat-network-webhook-deploy:v0.0.0
docker push harborlocal.hxstarrys.me/cnrancher/rancher-flat-network-webhook-deploy:v0.0.0

docker tag  rancher-flat-network-cni harborlocal.hxstarrys.me/cnrancher/rancher-flat-network-cni:v0.0.0
docker push harborlocal.hxstarrys.me/cnrancher/rancher-flat-network-cni:v0.0.0

helm upgrade --install rancher-flat-network-operator-crd ./charts/rancher-flat-network-crd

helm uninstall rancher-flat-network || true
sleep 1
helm upgrade --install rancher-flat-network \
    --set global.cattle.systemDefaultRegistry='harborlocal.hxstarrys.me' \
    --set flatNetworkOperator.replicas=0 \
    --set clusterType=K3s \
    ./charts/rancher-flat-network
