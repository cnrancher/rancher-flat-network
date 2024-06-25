#!/bin/bash

# Always exit on errors.
set -e

CNI_CONF_DIR="/host/etc/cni/net.d"
CNI_BIN_DIR="/host/opt/cni/bin"

cp -f /opt/cni/bin/* $CNI_BIN_DIR/

echo "Entering sleep... (success)"

# Sleep forever.
while sleep 3600000; do :; done
