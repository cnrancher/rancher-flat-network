#!/bin/bash

set -euo pipefail

kubectl delete ValidatingWebhookConfiguration flatnetwork-subnets-validating-config
kubectl delete svc flatnetwork-webhook-svc -n kube-system
kubectl delete secret flatnetwork-webhook-certs -n kube-system
kubectl delete csr flatnetwork-webhook-svc.kube-system 2> /dev/null || true
