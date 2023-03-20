FROM alpine

COPY firecracker /
COPY openfaas_hypervisor /
COPY cni /cni
COPY microvms /microvms

CMD ["/openfaas_hypervisor"]