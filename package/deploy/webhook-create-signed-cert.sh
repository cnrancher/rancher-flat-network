#!/bin/bash

# FYI:
#   1. https://kubernetes.io/docs/tasks/administer-cluster/certificates/
#   2. https://gist.github.com/tirumaraiselvan/b7eb1831d25dd9d59a785c11bd46c84b

set -euo pipefail

service=${service:-"rancher-flat-network-webhook-svc"}
secret=${secret:-"rancher-flat-network-webhook-certs"}
namespace=${namespace:-"kube-system"}

cd /certs

# 8760h == 365d
cat > ca-config.json <<EOF
{
  "signing": {
    "default": {
      "expiry": "8760h"
    },
    "profiles": {
      "rancher-flat-network-webhook-server": {
        "usages": ["signing", "key encipherment", "server auth", "client auth"],
        "expiry": "8760h"
      }
    }
  }
}
EOF

cat > ca-csr.json <<EOF
{
  "CN": "Rancher FlatNetwork Operator Webhook CA",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "CN",
      "L": "Shenyang",
      "O": "SUSE",
      "OU": "SUSE Rancher CA",
      "ST": "LiaoNing"
    }
  ]
}
EOF

cfssl gencert -initca ca-csr.json | cfssljson -bare ca

# Results: ca-key.pem ca.pem

cat > server-csr.json <<EOF
{
  "CN": "Rancher FlatNetwork Operator Webhook Cert",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "hosts": [
    "${service}",
    "${service}.${namespace}",
    "${service}.${namespace}.svc",
    "${service}.${namespace}.svc.cluster",
    "${service}.${namespace}.svc.cluster.local"
  ],
  "names": [
    {
      "C": "CN",
      "L": "Shenyang",
      "O": "SUSE",
      "OU": "SUSE Rancher",
      "ST": "Liaoning"
    }
  ]
}
EOF

cfssl gencert \
    -ca=ca.pem \
    -ca-key=ca-key.pem \
    -config=ca-config.json \
    -profile=rancher-flat-network-webhook-server \
    server-csr.json | cfssljson -bare server

# Results: server-key.pem server.pem

# Rotate TLS secret for server
kubectl delete -n ${namespace} secret ${secret} &> /dev/null || true
kubectl create -n ${namespace} secret tls ${secret} --cert=server.pem --key=server-key.pem
