all: firecracker microvm-files unikernel-files container-files openfaas_hypervisor docker-build-microvm docker-build-unikernel docker-build-container

run-microvm:
	docker run -it -p8080:8080 --privileged openfaas-hypervisor:microvm

run-unikernel:
	docker run -it -p8080:8080 --privileged openfaas-hypervisor:unikernel

run-container:
	docker run -it -p8080:8080 --privileged openfaas-hypervisor:container

firecracker:
	./install_firecracker.sh

microvm-files:
	$(MAKE) -C microvms all

unikernel-files:
	$(MAKE) -C unikernels all

container-files:
	$(MAKE) -C containers all

openfaas_hypervisor: openfaas_hypervisor.go pkg/*
	CGO_ENABLED=0 go build openfaas_hypervisor.go

docker-build-microvm:
	docker build -f Dockerfile.microvm -t openfaas-hypervisor:microvm .

docker-build-unikernel:
	docker build -f Dockerfile.unikernel -t openfaas-hypervisor:unikernel .

docker-build-container:
	docker build -f Dockerfile.container -t openfaas-hypervisor:container .

docker-push-microvm: docker-build-microvm
	docker tag openfaas-hypervisor:microvm public.ecr.aws/t7r4r6l6/openfaas-hypervisor:microvm
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:microvm

docker-push-unikernel: docker-build-unikernel
	docker tag openfaas-hypervisor:unikernel public.ecr.aws/t7r4r6l6/openfaas-hypervisor:unikernel
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:unikernel

docker-push-container: docker-build-container
	docker tag openfaas-hypervisor:container public.ecr.aws/t7r4r6l6/openfaas-hypervisor:container
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:container

clean:
	rm -f openfaas_hypervisor
	$(MAKE) -C microvms clean
	$(MAKE) -C unikernels clean
	$(MAKE) -C containers clean
