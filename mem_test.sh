for pid in $(pstree -pT $1 | grep -o "([0-9]*)" | sed 's/(//' | sed 's/)//'); 
do 
    sudo ./wss.pl $pid 0.1
done
