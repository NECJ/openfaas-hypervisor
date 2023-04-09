sudo kill $(pgrep openfaas_hyperv) &>/dev/null

sudo kill $(pgrep firecracker) &>/dev/null
sudo kill $(pgrep qemu) &>/dev/null
for id in $(sudo runsc list | cut -d ' ' -f1); 
do 
    sudo runsc kill $id
done

sudo ip link set dev ofhbr down &>/dev/null
sudo brctl delbr ofhbr &>/dev/null

for veth in $(ip addr | grep "ofhtap" | cut -d' ' -f2 | sed 's/://'); 
do 
    sudo ip link set dev $veth down &>/dev/null
    sudo ip tuntap del dev $veth mode tap &>/dev/null
done

sudo rm -r /tmp/openfaas-hypervisor-*
for i in $(seq 0 9);
do
       for j in $(seq 0 9);
       do
               sudo rm -r /tmp/openfaas-hypervisor-vm${i}${j}*
       done
done