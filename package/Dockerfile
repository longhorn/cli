FROM registry.suse.com/bci/bci-base:15.6

RUN zypper -n ref && \
    zypper update -y

RUN zypper -n install jq && \
    rm -rf /var/cache/zypp/*

COPY bin /usr/local/bin/
COPY spdk /spdk

CMD ["longhornctl"]
