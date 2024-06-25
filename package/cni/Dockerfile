FROM registry.suse.com/bci/bci-base:15.6

ARG ARCH=amd64
ARG STATIC_IPAM_VERSION=v1.5.1

RUN zypper -n install vim curl tar gzip && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    mkdir -p /tmp/bin /opt/cni/bin && \
    curl -sLf https://github.com/containernetworking/plugins/releases/download/${STATIC_IPAM_VERSION}/cni-plugins-linux-${ARCH}-${STATIC_IPAM_VERSION}.tgz | tar xvzf - -C /tmp/bin && \
    mv /tmp/bin/static /opt/cni/bin/static-ipam && \
    rm -rf /tmp/*

ADD package/cni/entrypoint.sh /
ADD package/cni/cni-loglevel.conf /
ADD bin/rancher-flat-network-cni /opt/cni/bin/

ENTRYPOINT ["/entrypoint.sh"]
