FROM gliderlabs/alpine:3.2

# Necessary depedencies
RUN apk --update add haproxy bash curl tar

# Install S6 from static bins
RUN cd / && curl -L https://github.com/just-containers/skaware/releases/download/v1.17.1/s6-eeb0f9098450dbe470fc9b60627d15df62b04239-linux-amd64-bin.tar.gz | tar -xvzf -

# Set up Sidecar
ADD sidecar /sidecar/sidecar
ADD sidecar.toml /sidecar/sidecar.toml
ADD views /sidecar/views
ADD s6 /etc

EXPOSE 7777

CMD ["/bin/s6-svscan", "/etc/services"]
