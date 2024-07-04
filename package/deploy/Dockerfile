FROM registry.suse.com/bci/bci-base:15.6

ARG ARCH=amd64
ARG K8S_VERSION=v1.29.6

RUN zypper -n install curl tar gzip ca-certificates openssl && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    curl -sLf https://storage.googleapis.com/kubernetes-release/release/${K8S_VERSION}/bin/linux/${ARCH}/kubectl > /bin/kubectl && \
    chmod +x /bin/kubectl && \
    mkdir -p /crds /certs

COPY --from=docker.hxstarrys.me/cnrancher/hardened-cloudflare-cfssl:v1.6.3-rancher1 /opt/bin/ /bin/

WORKDIR /

COPY package/deploy/*.sh /
COPY package/deploy/*.yaml /

ENTRYPOINT ["/entrypoint.sh"]
