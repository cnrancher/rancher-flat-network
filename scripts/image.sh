#!/bin/bash

set -euxo pipefail

cd $(dirname $0)/../
WORKINGDIR=$(pwd)

# TODO: DEBUG
./scripts/build.sh

docker build -t flat-network-operator -f ./package/operator/Dockerfile .
docker build -t flat-network-webhook-deploy -f ./package/webhook-deploy/Dockerfile .

# exit 0

# TODO: DEBUG
docker tag  flat-network-operator harborlocal.hxstarrys.me/cnrancher/flat-network-operator:v0.0.0
docker push harborlocal.hxstarrys.me/cnrancher/flat-network-operator:v0.0.0

docker tag  flat-network-webhook-deploy harborlocal.hxstarrys.me/cnrancher/flat-network-webhook-deploy:v0.0.0
docker push harborlocal.hxstarrys.me/cnrancher/flat-network-webhook-deploy:v0.0.0
