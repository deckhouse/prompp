ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="The Deckhouse Prom++ Authors"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/prometheus        /bin/prometheus
COPY .build/${OS}-${ARCH}/promtool          /bin/promtool
COPY documentation/examples/prometheus.yml  /etc/prometheus/prometheus.yml
COPY console_libraries/                     /usr/share/prometheus/console_libraries/
COPY consoles/                              /usr/share/prometheus/consoles/
COPY LICENSE                                /LICENSE
COPY NOTICE                                 /NOTICE
COPY npm_licenses.tar.bz2                   /npm_licenses.tar.bz2

WORKDIR /prometheus
RUN ln -s /usr/share/prometheus/console_libraries /usr/share/prometheus/consoles/ /etc/prometheus/ && \
    chown -R nobody:nobody /etc/prometheus /prometheus
RUN ln -s /bin/prometheus /bin/prompp

ENV GOGC="30"
ENV MALLOC_CONF="background_thread:true,tcache_max:4096,dirty_decay_ms:5000,muzzy_decay_ms:5000"
ENV PROMPP_FEATURES=""

USER       nobody
EXPOSE     9090
VOLUME     [ "/prometheus" ]
ENTRYPOINT [ "/bin/prompp" ]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus", \
             "--web.console.libraries=/usr/share/prometheus/console_libraries", \
             "--web.console.templates=/usr/share/prometheus/consoles" ]
