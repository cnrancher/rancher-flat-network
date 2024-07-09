#!/bin/bash

# FYI: https://kubernetes.io/docs/tasks/administer-cluster/certificates/

set -euo pipefail

cd /certs

type easyrsa3/easyrsa
type dig
type kubectl

service=${service:-"rancher-flat-network-webhook-svc"}
secret=${secret:-"rancher-flat-network-webhook-certs"}
namespace=${namespace:-"cattle-flat-network"}

MASTER_IP=${KUBERNETES_SERVICE_HOST:-}
if [[ -z "${MASTER_IP:-}" ]]; then
    MASTER_IP=$(dig +short kubernetes.default.svc.cluster.local | head -n 1)
fi

if [[ -z ${MASTER_IP:-} ]]; then
    echo "Failed to get Cluster master IP"
    exit 1
fi

pushd easyrsa3

echo "Init PKI..."
echo yes | ./easyrsa init-pki
echo "-------------------------------------------------------"

echo "Build CA..."
./easyrsa --batch "--req-cn=${MASTER_IP}@`date +%s`" build-ca nopass
echo "-------------------------------------------------------"

echo "Generate cert..."
echo yes | ./easyrsa --subject-alt-name="IP:${MASTER_IP},"\
"DNS:${service},"\
"DNS:${service}.${namespace},"\
"DNS:${service}.${namespace}.svc,"\
"DNS:${service}.${namespace}.svc.cluster,"\
"DNS:${service}.${namespace}.svc.cluster.local" \
    --days=365 \
    build-server-full server nopass

cp pki/ca.crt ../
cp pki/issued/server.crt ../
cp pki/private/server.key ../
rm -r pki

popd

echo '----------------------------'
ls -alh
echo '----------------------------'

# Rotate TLS secret for server
echo "Applying TLS secret..."
if kubectl -n ${namespace} get secret ${secret} &> /dev/null; then
    kubectl delete -n ${namespace} secret ${secret}
fi
kubectl create -n ${namespace} secret tls ${secret} --cert=server.crt --key=server.key
