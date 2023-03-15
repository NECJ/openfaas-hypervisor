sudo brctl addbr virbr0
sudo ip a a  123.123.0.1/24 dev virbr0
sudo ip l set dev virbr0 up

# create tap
sudo ip tuntap add dev tap0 mode tap
# attach tap to bridge
sudo ip link set dev tap0 master virbr0
# give tap ip
sudo ip a a  123.123.0.2/24 dev tap0
# This needed?
sudo ip l set dev virbr0 up