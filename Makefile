all: firecracker microvm-files openfaas_hypervisor docker-build

firecracker:
	./install_firecracker.sh

microvm-files:
	$(MAKE) -C microvm all

openfaas_hypervisor:
	CGO_ENABLED=0 go build openfaas_hypervisor.go

docker-build:
	docker build -t openfaas-hypervisor .

docker-push: docker-build
	docker tag openfaas-hypervisor:latest public.ecr.aws/t7r4r6l6/openfaas-hypervisor:latest
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:latest

clean:
	rm -f openfaas_hypervisor
	$(MAKE) -C microvm clean