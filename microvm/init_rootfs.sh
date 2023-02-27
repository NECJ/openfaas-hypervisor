#!/bin/sh 

apk add openrc
apk add util-linux
apk add curl

# Set up a terminal on the serial console (ttyS0):
echo "agetty_options=\"--autologin root --noclear\"" > /etc/conf.d/agetty-autologin
ln -s agetty /etc/init.d/agetty-autologin.ttyS0
echo ttyS0 > /etc/securetty
rc-update add agetty-autologin.ttyS0 default

# Make sure special file systems are mounted on boot:
rc-update add devfs boot
rc-update add procfs boot
rc-update add sysfs boot

rc-update add local default

# Then, copy the newly configured system to the rootfs image:
for d in bin etc lib root sbin usr; do tar c "/$d" | tar x -C /my-rootfs; done

# The above command may trigger the following message:
# tar: Removing leading "/" from member names
# However, this is just a warning, so you should be able to
# proceed with the setup process.

for dir in dev proc run sys var; do mkdir /my-rootfs/${dir}; done