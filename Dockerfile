FROM alpine

# Give permission for QMEU to use bridge
RUN mkdir /etc/qemu
RUN echo "allow virbr0" >> /etc/qemu/bridge.conf

RUN apk add qemu-system-x86_64
COPY openfaas_hypervisor /
COPY unikernel/build/httpreply_kvm-x86_64 /unikernel/build/httpreply_kvm-x86_64

CMD ["/openfaas_hypervisor"]