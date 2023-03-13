package IpIterator

import "net"

type IpIterator struct {
	net.IP
}

func ParseIP(s string) IpIterator {
	return IpIterator{net.ParseIP(s).To4()}
}

func (i IpIterator) Next() {
	if i.IP[3] != 255 {
		i.IP[3]++
	} else if i.IP[2] != 255 {
		i.IP[2]++
		i.IP[3] = 0
	} else if i.IP[1] != 255 {
		i.IP[1]++
		i.IP[2] = 0
		i.IP[3] = 0
	} else if i.IP[0] != 255 {
		i.IP[0]++
		i.IP[1] = 0
		i.IP[2] = 0
		i.IP[3] = 0
	} else {
		i.IP[0] = 0
		i.IP[1] = 0
		i.IP[2] = 0
		i.IP[3] = 0
	}
}
