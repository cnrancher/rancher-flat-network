#!/bin/bash

set -euo pipefail

cd $(dirname $0)/

CNI_CONF_DIR="/host/etc/cni/net.d"
CNI_BIN_DIR="/host/opt/cni/bin"

CNI_LOGLEVEL_CONF="/cni-loglevel.conf"
CNI_LOG_FILE="/var/log/rancher-flat-network-cni.log"

cp -f /opt/cni/bin/* $CNI_BIN_DIR/

echo "CNI Loglevel: $(cat $CNI_LOGLEVEL_CONF)"
echo "Start watching CNI log: ${CNI_LOG_FILE}"

while true
do
    tail -f ${CNI_LOG_FILE} || true
    sleep 1
done
