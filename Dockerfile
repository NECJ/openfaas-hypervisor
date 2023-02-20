FROM alpine:latest

COPY install_firecracker.sh .
RUN /bin/sh install_firecracker.sh

COPY ./ /

CMD ["/firecracker", "--api-sock", "/tmp/firecracker.socket", "--config-file", "/firecracker_config.yml"]