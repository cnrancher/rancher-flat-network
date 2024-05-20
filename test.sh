#!/bin/bash

set -ex

FLAT_IF=${FLAT_IF:-ens5}
BRIDGE_NAME=${FLAT_BRIDGE:-flatbr0}
MTU=${MTU:-1500}

TEST_BRIDGE=$(ip addr show $BRIDGE_NAME | grep 'inet\b' | awk '{print $2}')
if [ ! -z $TEST_BRIDGE ]; then
    exit 0
fi

# Set varibles
FLAT_IF_IP=$(ip addr show $FLAT_IF | grep 'inet\b' | awk '{print $2}')
FLAT_IF_MAC=$(ip addr show $FLAT_IF | grep ether | awk '{print $2}')
BRIDGE_IP=${FLAT_IF_IP}
BRIDGE_MAC=${FLAT_IF_MAC}
GW_IP=$(ip route show | grep default | awk '{print $3}')

ip link add ${BRIDGE_NAME} type bridge || true
ip link set ${BRIDGE_NAME} address ${BRIDGE_MAC}
ip addr del ${FLAT_IF_IP} dev ${FLAT_IF}
ip addr add ${BRIDGE_IP} brd + dev ${BRIDGE_NAME}
ip link set dev ${BRIDGE_NAME} up
ip link set dev ${FLAT_IF} master ${BRIDGE_NAME}
ip link set dev ${BRIDGE_NAME} mtu ${MTU}

if [ -z $(ip route show | grep default | awk '{print $3}')  ]; then
    ip route add default via ${GW_IP}
fi
