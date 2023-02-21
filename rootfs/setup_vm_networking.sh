#!/bin/sh

ifconfig eth0 up
ip addr add dev eth0 172.17.0.20/16
ip route add default via 172.17.0.1
echo "nameserver 8.8.8.8" > /etc/resolv.conf