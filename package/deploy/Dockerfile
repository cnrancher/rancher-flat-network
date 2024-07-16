FROM registry.suse.com/bci/bci-base:15.6

ARG TARGETARCH
ARG KUBECTL_VERSION=v1.30.2
RUN zypper -n install curl tar gzip ca-certificates bind-utils awk && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    mkdir -p /certs && \
    curl -sLf https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl > /bin/kubectl && \
    curl -LO https://dl.k8s.io/easy-rsa/easy-rsa.tar.gz && \
    tar xzf easy-rsa.tar.gz && mv easy-rsa-master/easyrsa3 /certs/ && rm -rf ./easy-rsa-master && \
    chmod +x /bin/kubectl /certs/easyrsa3/easyrsa

WORKDIR /

COPY package/deploy/*.sh /
COPY package/deploy/*.yaml /

ENTRYPOINT ["/entrypoint.sh"]
