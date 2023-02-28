FROM alpine

COPY firecracker /
COPY openfaas_hypervisor /
COPY cni /cni
COPY microvm/rootfs.ext4 /microvm/rootfs.ext4
COPY microvm/vmlinux /microvm/vmlinux

CMD ["/openfaas_hypervisor"]