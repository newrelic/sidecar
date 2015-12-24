FROM gliderlabs/alpine:3.2

RUN apk --update add haproxy bash

ADD sidecar /sidecar/sidecar
ADD sidecar.toml /sidecar/sidecar.toml
ADD views /sidecar/views
ADD start.sh /start.sh

EXPOSE 7777
WORKDIR /sidecar

ENTRYPOINT ["/start.sh"]
