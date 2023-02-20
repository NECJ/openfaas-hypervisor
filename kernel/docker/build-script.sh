#!/bin/sh

# rpm -qa | grep binutils
cd linux-5.10/
cp ../.config ./
make vmlinux -j8
mv vmlinux /output