FROM registry.suse.com/bci/bci-base:15.4

RUN zypper -n install git && \
    rm -rf /var/cache/zypp/*

ENV SPDK_COMMIT_ID 4542f9b14010fc7192d42e05adaafc2ae863bac5
RUN cd /usr/src && \
    git clone https://github.com/longhorn/spdk.git /spdk && \
    cd /spdk && \
    git checkout ${SPDK_COMMIT_ID}

COPY bin /usr/local/sbin/

VOLUME /usr/local/sbin

CMD ["longhorn-preflight"]