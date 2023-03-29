all: firecracker microvm-files unikernel-files openfaas_hypervisor docker-build-microvm docker-build-unikernel

run-unikernel:
	docker run -it -p8080:8080 --privileged openfaas-hypervisor:unikernel

run-microvm:
	docker run -it -p8080:8080 --privileged openfaas-hypervisor:microvm

firecracker:
	./install_firecracker.sh

microvm-files:
	$(MAKE) -C microvms all

unikernel-files:
	$(MAKE) -C unikernels all

openfaas_hypervisor: openfaas_hypervisor.go pkg/AtomicIpIterator.go pkg/AtomicIterator.go pkg/Stats.go pkg/VmPool.go
	CGO_ENABLED=0 go build openfaas_hypervisor.go

docker-build-microvm:
	docker build -f Dockerfile.microvm -t openfaas-hypervisor:microvm .

docker-build-unikernel:
	docker build -f Dockerfile.unikernel -t openfaas-hypervisor:unikernel .

docker-push-microvm: docker-build-microvm
	docker tag openfaas-hypervisor:microvm public.ecr.aws/t7r4r6l6/openfaas-hypervisor:microvm
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:microvm

docker-push-unikernel: docker-build-unikernel
	docker tag openfaas-hypervisor:unikernel public.ecr.aws/t7r4r6l6/openfaas-hypervisor:unikernel
	docker push public.ecr.aws/t7r4r6l6/openfaas-hypervisor:unikernel

clean:
	rm -f openfaas_hypervisor
	$(MAKE) -C microvms clean
	$(MAKE) -C unikernels clean
