FROM registry.suse.com/bci/bci-base:15.7

ARG TARGETARCH
ARG STATIC_IPAM_VERSION=v1.8.0
RUN zypper -n install vim curl tar gzip && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    mkdir -p /tmp/bin /opt/cni/bin /etc/rancher/flat-network/ && \
    curl -sLf https://github.com/containernetworking/plugins/releases/download/${STATIC_IPAM_VERSION}/cni-plugins-linux-${TARGETARCH}-${STATIC_IPAM_VERSION}.tgz | tar xvzf - -C /tmp/bin && \
    mv /tmp/bin/static /opt/cni/bin/static-ipam && \
    rm -rf /tmp/*

COPY package/cni/entrypoint.sh /
COPY package/cni/cni-loglevel.conf /etc/rancher/flat-network/
COPY dist/rancher-flat-network-cni_linux_${TARGETARCH}*/ /opt/cni/bin/

ENTRYPOINT ["/entrypoint.sh"]
