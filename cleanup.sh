sudo kill $(pgrep openfaas_hyperv) &>/dev/null

sudo kill $(pgrep qemu) &>/dev/null

for veth in $(ip addr | grep "veth" | cut -d' ' -f2 | sed 's/://' | sed 's/@if2//'); 
do 
    sudo ifconfig $veth down
    sudo ip link del $veth
done

sudo rm -r /tmp/openfaas-hypervisor-vm*