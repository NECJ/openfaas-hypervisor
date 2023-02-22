firecracker:
	./install_firecracker.sh

network: bridge tap

network_clean: bridge_clean tap_clean

tap:
	ip tuntap add veth123 mode tap
	brctl addif funcbridge veth123
	# ifconfig veth123 up
	ip link set dev veth123 up

tap_clean:
	ip tuntap del veth123 mode tap

bridge:
	ip link add name funcbridge type bridge
	ip addr add 172.20.0.1/16 dev funcbridge
	ip link set dev funcbridge up
	sysctl -w net.ipv4.ip_forward=1
	iptables -t nat -A POSTROUTING -o ens18 -j MASQUERADE
	iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
	iptables -A FORWARD -i veth123 -o ens18 -j ACCEPT

bridge_clean:
	ip link del name funcbridge type bridge
	iptables -t nat -D POSTROUTING -o ens18 -j MASQUERADE
	iptables -D FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
	iptables -D FORWARD -i veth123 -o ens18 -j ACCEPT