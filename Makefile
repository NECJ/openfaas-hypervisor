all: firecracker microvm-files openfaas_hypervisor docker-build

run:
	docker run -it -p8080:8080 --privileged openfaas-hypervisor:microvm

firecracker:
	./install_firecracker.sh

microvm-files:
	$(MAKE) -C microvms all

openfaas_hypervisor: openfaas_hypervisor.go pkg/Stats.go pkg/VmPool.go
	CGO_ENABLED=0 go build openfaas_hypervisor.go

docker-build:
	docker build -t openfaas-hypervisor:microvm .

docker-push: docker-build
	docker tag openfaas-hypervisor:microvm public.ecr.aws/t7r4r6l6/openfaas-hypervisor:microvm
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:microvm

clean:
	rm -f openfaas_hypervisor
	$(MAKE) -C microvms clean