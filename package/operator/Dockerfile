FROM registry.suse.com/bci/bci-base:15.6

ARG TARGETARCH
ARG LOGLEVEL_VERSION=v0.1.6
RUN zypper update -y && \
    zypper in -y -f vim curl tar gzip && \
    zypper clean -a && \
    rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    curl -sLf https://github.com/rancher/loglevel/releases/download/${LOGLEVEL_VERSION}/loglevel-${TARGETARCH}-${LOGLEVEL_VERSION}.tar.gz | tar xvzf - -C /usr/bin

RUN useradd --uid 1007 rancher-flat-network-operator
ENV KUBECONFIG /home/rancher-flat-network-operator/.kube/config
COPY dist/rancher-flat-network-operator_linux_${TARGETARCH}*/ /usr/bin/
COPY package/operator/entrypoint.sh /

USER 1007
ENTRYPOINT ["/entrypoint.sh"]
CMD [ "rancher-flat-network-operator" ]
