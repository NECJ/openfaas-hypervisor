sudo kill $(pgrep openfaas_hyperv) &>/dev/null

sudo ip link set dev ofhbr down &>/dev/null
sudo brctl delbr ofhbr &>/dev/null
for i in $(seq 1 100)
do
    sudo ip link set dev ofhtap$i down &>/dev/null
    sudo ip tuntap del dev ofhtap$i mode tap &>/dev/null
done