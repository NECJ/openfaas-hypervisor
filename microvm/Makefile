all: server vmlinux rootfs.ext4

server:
	CGO_ENABLED=0 go build server.go

vmlinux:
	./build_kernel.sh

rootfs.ext4:
	./build_rootfs.sh