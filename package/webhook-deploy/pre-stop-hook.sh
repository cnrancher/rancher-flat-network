#!/bin/bash

set -euo pipefail

kubectl delete ValidatingWebhookConfiguration flatnetwork-subnets-validating-config
kubectl delete svc flatnetwork-operator-webhook-svc -n kube-system
kubectl delete secret flatnetwork-operator-webhook-certs -n kube-system
kubectl delete csr flatnetwork-operator-webhook-svc.kube-system 2> /dev/null || true
