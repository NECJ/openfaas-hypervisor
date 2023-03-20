FROM alpine

# Give permission for QMEU to use bridge
RUN mkdir /etc/qemu
RUN echo "allow virbr0" >> /etc/qemu/bridge.conf

RUN apk add qemu-system-x86_64
COPY openfaas_hypervisor /
COPY unikernels /unikernels
RUN find /unikernels -type f \! -name "httpreply_kvm-x86_64" -print | xargs rm -rf

CMD ["/openfaas_hypervisor"]