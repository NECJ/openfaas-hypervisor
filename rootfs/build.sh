#!/bin/sh

dd if=/dev/zero of=rootfs.ext4 bs=1M count=50
mkfs.ext4 rootfs.ext4
mkdir /tmp/my-rootfs
sudo mount rootfs.ext4 /tmp/my-rootfs

# gcc main.c -o main --static
# sudo mkdir /tmp/my-rootfs/sbin
# sudo mv main /tmp/my-rootfs/sbin/init

docker run -it --rm -v $(pwd)/create-alpine-fs.sh:/create-alpine-fs.sh -v /tmp/my-rootfs:/my-rootfs alpine /create-alpine-fs.sh

sudo umount /tmp/my-rootfs
rm -r /tmp/my-rootfs