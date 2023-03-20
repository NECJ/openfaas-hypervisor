#!/bin/sh 

apk add openrc

rc-update add local default

# Copy the newly configured system to the rootfs image:
for d in bin etc lib sbin usr; do tar c "/$d" | tar x -C /my-rootfs; done

# The above command may trigger the following message:
# tar: Removing leading "/" from member names
# However, this is just a warning, so you should be able to
# proceed with the setup process.

for dir in dev proc run sys var; do mkdir /my-rootfs/${dir}; done