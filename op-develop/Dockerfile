FROM alpine:3.20.3
ARG GOVERSION=1.23.4
WORKDIR /src

RUN apk update && apk add curl binutils-gold gcc g++ pcre-dev ragel openjdk8 maven linux-headers make jq bash nodejs npm
RUN export ARCH=$(uname -m | sed 's/aarch/arm/;s/x86_/amd/') && \
    cd /tmp && \
    curl -sO https://dl.google.com/go/go${GOVERSION}.linux-$ARCH.tar.gz && \
    tar -C /usr/local -xzf go${GOVERSION}.linux-$ARCH.tar.gz

RUN cd /tmp && curl -sLJO https://sourceforge.net/projects/pcre/files/pcre/8.45/pcre-8.45.tar.gz/download && \
    tar -xzf pcre-8.45.tar.gz && \
    cd pcre-8.45 && \
    CFLAGS="-fno-stack-protector" ./configure --prefix=/usr/local && make install

ENV PATH=$PATH:/usr/local/go/bin:/root/go/bin
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
RUN go install github.com/prometheus/promu@latest

