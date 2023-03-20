all: server rootfs.ext4

server: server.c
	gcc -static -O3 server.c -o server

rootfs.ext4: server
	dd if=/dev/zero of=rootfs.ext4 bs=1M count=50
	mkfs.ext4 rootfs.ext4
	mkdir /tmp/my-rootfs
	sudo mount rootfs.ext4 /tmp/my-rootfs
	docker run -it --rm \
		-v $(shell pwd)/init_rootfs.sh:/init_rootfs.sh \
		-v /tmp/my-rootfs:/my-rootfs \
		-v $(shell pwd)/ready.sh:/etc/local.d/ready.start \
		-v $(shell pwd)/server:/bin/server \
		alpine /init_rootfs.sh
	sudo umount /tmp/my-rootfs
	rm -r /tmp/my-rootfs

clean:
	rm -f rootfs.ext4 server