SUBDIRS := $(wildcard */.)

all: fs vmlinux

vmlinux:
	docker build . -t microvm-build-kernel -f microvm-build-kernel.Dockerfile
	docker run -it -v $(shell pwd):/output microvm-build-kernel

fs: 
	for dir in $(SUBDIRS); do \
	$(MAKE) -C $$dir all; \
	done

clean:
	rm -f vmlinux
	for dir in $(SUBDIRS); do \
	$(MAKE) -C $$dir clean; \
	done