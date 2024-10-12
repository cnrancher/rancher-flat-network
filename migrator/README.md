# Rancher FlatNetwork Migrator

Backup/Restore/Migrate CRD resources & workloads from `macvlan.cluster.cattle.io` to `flatnetwork.pandaria.io`.

## Specifications

Rancher Macvlan V1 CRD `macvlan.cattle.cluster.io`.

Pod template annotation keys used by V1:

- `macvlan.pandaria.cattle.io/ip`
- `macvlan.pandaria.cattle.io/subnet`
- `macvlan.pandaria.cattle.io/mac`
- `macvlan.panda.io/ingress`
- `macvlan.panda.io/macvlanService`
- `macvlan.panda.io/ipv6to4`
- `macvlan.panda.io/ipDelayReuseTimestamp`

----

Rancher FlatNetwork V2 CRD `flatnetwork.pandaria.io`.

Pod template annotation keys after migrated to V2:

- `flatnetwork.pandaria.io/ip`
- `flatnetwork.pandaria.io/subnet`
- `flatnetwork.pandaria.io/mac`
- `flatnetwork.pandaria.io/ingress`
- `flatnetwork.pandaria.io/flatNetworkService`
- `flatnetwork.pandaria.io/ipv6to4`

## Commands

- `backup`: Backup V1 & V2 subnet CRD resources.
- `restore`: Restore V1 & V2 subnet CRD resources.
- `migrate`: Migrate V1 subnet & workloads to V2.
- `clean`: Delete V1 CRD resources.
