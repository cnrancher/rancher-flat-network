#!/bin/bash

set -euo pipefail

echo "Generating flatnetwork-operator webhook TLS secrets..."
./webhook-create-signed-cert.sh
echo

if [[ ${ROLLOUT_FLATNETWORK_DEPLOYMENT:-} = true ]] && kubectl get deployment flatnetwork-operator &> /dev/null
then
    echo "Restart flatnetwork-operator deployment..."
    kubectl -n kube-system rollout restart deployment/flatnetwork-operator
    echo
fi

echo "Deploying flatnetwork operator validating webhook configurations..."
cat ./validating-webhook.yaml | /webhook-patch-ca-bundle.sh | kubectl apply -f -
echo

echo "Successfully setup flatnetwork-operator webhook configurations..."
exit 0
