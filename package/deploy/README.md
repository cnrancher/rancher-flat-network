# Rancher flat-network deploy Dockerfile

This container is used in following parts:
- Jobs/CronJob for rollout webhook TLS certificates.
- Init container for Operator to wait for webhook tls certificate generated.
- Init container for multus DaemonSet pod for cleanup auto generated config.
