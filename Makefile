all: firecracker unikernel-files openfaas_hypervisor docker-build

run:
	docker run -it -p8080:8080 --privileged openfaas-hypervisor:unikernel

firecracker:
	./install_firecracker.sh

unikernel-files:
	$(MAKE) -C unikernel all

openfaas_hypervisor: openfaas_hypervisor.go
	CGO_ENABLED=0 go build openfaas_hypervisor.go

docker-build:
	docker build -t openfaas-hypervisor:unikernel .

docker-push: docker-build
	docker tag openfaas-hypervisor:unikernel public.ecr.aws/t7r4r6l6/openfaas-hypervisor:unikernel
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:unikernel

clean:
	rm -f openfaas_hypervisor
	$(MAKE) -C unikernel clean