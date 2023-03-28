sudo kill $(pgrep openfaas_hyperv) &>/dev/null

sudo kill $(pgrep firecracker) &>/dev/null

sudo ip link set dev ofhbr down &>/dev/null
sudo brctl delbr ofhbr &>/dev/null

for veth in $(ip addr | grep "ofhtap" | cut -d' ' -f2 | sed 's/://'); 
do 
    sudo ip link set dev $veth down &>/dev/null
    sudo ip tuntap del dev $veth mode tap &>/dev/null
done

sudo rm -r /tmp/openfaas-hypervisor-vm*