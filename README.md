<div align="center">
  <h1>rancher-flat-network</h1>
  <p>
    <a href="https://github.com/cnrancher/rancher-flat-network/actions/workflows/ci.yaml"><img alt="CI" src="https://github.com/cnrancher/rancher-flat-network/actions/workflows/ci.yaml/badge.svg"></a>
    <a href="https://goreportcard.com/report/github.com/cnrancher/rancher-flat-network"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/cnrancher/rancher-flat-network"></a>
    <a href="https://github.com/cnrancher/rancher-flat-network/releases"><img alt="GitHub release" src="https://img.shields.io/github/v/release/cnrancher/rancher-flat-network?color=default&label=release&logo=github"></a>
    <a href="https://github.com/cnrancher/rancher-flat-network/releases"><img alt="GitHub pre-release" src="https://img.shields.io/github/v/release/cnrancher/rancher-flat-network?include_prereleases&label=pre-release&logo=github"></a>
    <img alt="License" src="https://img.shields.io/badge/License-Apache_2.0-blue.svg">
  </p>
</div>

Rancher Flat-Network Operator (based on [Rancher Wrangler](https://github.com/rancher/wrangler/)) & CNI plugin for managing
pods using the flat-networks (Macvlan/IPvlan).

## Features

### Operator

- [x] FlatNetwork subnet IPAM supports for both Macvlan & IPvlan.
- [x] Support for both IPv4 and IPv6 addresses.
- [x] IPAM supports custom specified IP address or allocate IP address automatically.
- [x] Custom IP Range to allocate IP address.
- [x] Auto create FlatNetwork headless ClusterIP service.
- [x] Leader election support to run operator & webhook server in multi-replicas (HA).
- [x] Update FlatNetwork Service Endpoints IP address.
- [x] Admission webhook server.

### CNI

- [X] Macvlan & IPvlan support.
- [X] CNI Spec 1.0.0 support.

### Migrator

- [x] Upgrade resource migrator from `macvlan.cluster.cattle.io` to `flatnetwork.pandaria.io`.

## Usage

Helm Chart of FlatNetwork V2: <https://github.com/cnrancher/pandaria-catalog/>.

Example workloads are available in [docs](./docs/) directory.

```console
$ kubectl apply -f ./docs/macvlan
$ kubectl apply -f ./docs/ipvlan
```

Environment variables for operator:

- `CATTLE_DEV_MODE`: Enable debug outputs and extend the leader election renew deadline & lease duration to support delve breakpoint debug operations, default `false`.
- `CATTLE_RESYNC_DEFAULT`: period to resync resources in minutes, default `600min` (10h).
- `CATTLE_ELECTION_LEASE_DURATION`: leader election lease duration, default `45s`.
- `CATTLE_ELECTION_RENEW_DEADLINE`: leader election renew deadline, default `30s`.
- `CATTLE_ELECTION_RETRY_PERIOD`: leader election retry period, default `2s`.
- `FLAT_NETWORK_CNI_ARP_POLICY`: CNI ARP Policy, default `arp_notify`, available `arp_notify`, `arping`.
- `FLAT_NETWORK_CLUSTER_CIDR`: Kubernetes config Cluster CIDR, default `10.42.0.0/16`.
- `FLAT_NETWORK_SERVICE_CIDR`: Kubernetes config Service CIDR, default `10.43.0.0/16`.

## License

Copyright 2025 SUSE Rancher

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
