FROM registry.suse.com/bci/bci-base:15.7

ARG TARGETARCH
ARG KUBECTL_VERSION=v1.33.4
RUN zypper -n install curl tar gzip ca-certificates bind-utils awk && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    mkdir -p /certs && \
    curl -sLf https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl > /bin/kubectl && \
    curl -LO https://dl.k8s.io/easy-rsa/easy-rsa.tar.gz && \
    tar xzf easy-rsa.tar.gz && mv easy-rsa-master/easyrsa3 /certs/ && rm -rf ./easy-rsa-master && \
    chmod +x /bin/kubectl /certs/easyrsa3/easyrsa && \
    kubectl help

WORKDIR /

COPY package/deploy/*.sh /
COPY package/deploy/*.yaml /

ENTRYPOINT ["/entrypoint.sh"]
