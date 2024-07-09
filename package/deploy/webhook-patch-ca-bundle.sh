#!/bin/bash

set -euo pipefail

export CA_BUNDLE=$(cat /certs/ca.crt | base64 -w0)

if command -v envsubst &> /dev/null; then
    envsubst
else
    sed -e "s|\${CA_BUNDLE}|${CA_BUNDLE}|g"
fi
