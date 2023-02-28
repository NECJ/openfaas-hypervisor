all: firecracker microvm-files openfaas_hypervisor

firecracker:
	./install_firecracker.sh

microvm-files:
	$(MAKE) -C microvm all

openfaas_hypervisor:
	CGO_ENABLED=0 go build openfaas_hypervisor.go

run: all
	sudo ./openfaas_hypervisor