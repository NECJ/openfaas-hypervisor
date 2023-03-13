package AtomicIpIterator

import (
	"net"
	"sync"
)

type AtomicIpIterator struct {
	ip    net.IP
	mutex *sync.Mutex
}

func ParseIP(s string) AtomicIpIterator {
	return AtomicIpIterator{net.ParseIP(s).To4(), &sync.Mutex{}}
}

func (i AtomicIpIterator) Next() string {
	i.mutex.Lock()
	if i.ip[3] != 255 {
		i.ip[3]++
	} else if i.ip[2] != 255 {
		i.ip[2]++
		i.ip[3] = 0
	} else if i.ip[1] != 255 {
		i.ip[1]++
		i.ip[2] = 0
		i.ip[3] = 0
	} else if i.ip[0] != 255 {
		i.ip[0]++
		i.ip[1] = 0
		i.ip[2] = 0
		i.ip[3] = 0
	} else {
		i.ip[0] = 0
		i.ip[1] = 0
		i.ip[2] = 0
		i.ip[3] = 0
	}
	i.mutex.Unlock()
	return i.ip.String()
}
