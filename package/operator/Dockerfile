FROM registry.suse.com/bci/bci-base:15.6

ARG LOGLEVEL_VERSION=v0.1.6
ARG ARCH=amd64

RUN zypper update -y && \
    zypper in -y -f vim curl tar gzip && \
    zypper clean -a && \
    rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    curl -sLf https://github.com/rancher/loglevel/releases/download/${LOGLEVEL_VERSION}/loglevel-${ARCH}-${LOGLEVEL_VERSION}.tar.gz | tar xvzf - -C /usr/bin

RUN useradd --uid 1007 flat-network-operator
ENV KUBECONFIG /home/flat-network-operator/.kube/config
COPY bin/flat-network-operator /usr/bin/
COPY package/operator/entrypoint.sh /

USER 1007
ENTRYPOINT ["/entrypoint.sh"]
CMD [ "flat-network-operator", "--debug" ]
