#!/bin/bash

set -euo pipefail

export CA_BUNDLE=$(cat /certs/ca.pem | base64 - | tr -d '\n')

if command -v envsubst &> /dev/null; then
    envsubst
else
    sed -e "s|\${CA_BUNDLE}|${CA_BUNDLE}|g"
fi
