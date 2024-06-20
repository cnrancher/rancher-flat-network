#!/bin/bash

set -euxo pipefail

cd $(dirname $0)/../
WORKINGDIR=$(pwd)

# Build linux image
export GOOS=linux

# TODO: DEBUG
go mod tidy
./scripts/build.sh

docker build -t flat-network-operator -f ./package/operator/Dockerfile .
docker build -t flat-network-webhook-deploy -f ./package/webhook-deploy/Dockerfile .

# exit 0

# TODO: DEBUG
docker tag  flat-network-operator 127.0.0.1:5010/cnrancher/flat-network-operator:v0.0.0
docker push 127.0.0.1:5010/cnrancher/flat-network-operator:v0.0.0

docker tag  flat-network-webhook-deploy 127.0.0.1:5010/cnrancher/flat-network-webhook-deploy:v0.0.0
docker push 127.0.0.1:5010/cnrancher/flat-network-webhook-deploy:v0.0.0

helm upgrade --install rancher-flat-network-operator-crd ./charts/rancher-flatnetwork-operator-crd

helm uninstall rancher-flat-network-operator || true
sleep 2
helm upgrade --install rancher-flat-network-operator \
    --set global.cattle.systemDefaultRegistry='127.0.0.1:5010' \
    ./charts/rancher-flatnetwork-operator
