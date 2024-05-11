FROM registry.suse.com/bci/bci-base:15.6

RUN zypper update -y && \
    zypper in -y -f vim && \
    zypper clean -a && \
    rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/*

RUN useradd --uid 1007 flat-network-operator
ENV KUBECONFIG /home/flat-network-operator/.kube/config
COPY bin/flat-network-operator /usr/bin/
USER 1007
ENTRYPOINT ["flat-network-operator"]
