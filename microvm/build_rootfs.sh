#!/bin/sh

dd if=/dev/zero of=rootfs.ext4 bs=1M count=50
mkfs.ext4 rootfs.ext4
mkdir /tmp/my-rootfs
sudo mount rootfs.ext4 /tmp/my-rootfs

docker run -it --rm \
    -v $(pwd)/init_rootfs.sh:/init_rootfs.sh \
    -v /tmp/my-rootfs:/my-rootfs \
    -v $(pwd)/ready.sh:/etc/local.d/ready.start \
    alpine /init_rootfs.sh

sudo umount /tmp/my-rootfs
rm -r /tmp/my-rootfs