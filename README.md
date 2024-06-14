# flat-network-operator

Rancher Flat-Network Operator based on [Wrangler](https://github.com/rancher/wrangler/).

## Features

- [x] FlatNetwork subnet IPAM supports for both Macvlan & IPvlan.
- [x] Support for both IPv4 and IPv6 addresses.
- [x] IPAM supports custom specified IP address or allocate IP address automatically.
- [x] Custom IP Range to allocate IP address.
- [x] Auto create FlatNetwork headless ClusterIP service.
- [x] Leader election support to run operator in multi-replicas (HA).
- [ ] Update FlatNetwork Service Endpoints IP address.
- [x] Admission webhook server.
- [ ] Upgrade resource migrator from `macvlan.cluster.cattle.io` to `flatnetwork.pandaria.io`.
helm

## Usage

To build and run flat-network operator manually:

1. Apply CRDs.

    ```console
    $ kubectl apply -f ./charts/flat-network-operator-crd/templates/crd.yaml
    ```

1. Build and run flat-network operator.

    ```console
    $ go build . && ./flat-network-operator
    ```

1. Launch another terminal to create example workloads.

    ```console
    $ kubectl apply -f ./docs/macvlan
    $ kubectl apply -f ./docs/ipvlan
    ```

## Environment variables

- `CATTLE_RESYNC_DEFAULT`: period to resync resources in minutes, default `600` (10h).
- `CATTLE_DEV_MODE`: extend the leader election renew deadline & lease duration to support delve breakpoint debug operations, default `false`.
- `CATTLE_ELECTION_LEASE_DURATION`: leader election lease duration, default `45s`.
- `CATTLE_ELECTION_RENEW_DEADLINE`: leader election renew deadline, default `30s`.
- `CATTLE_ELECTION_RETRY_PERIOD`: leader election retry period, default `2s`.

## License

Copyright 2024 SUSE Rancher

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
