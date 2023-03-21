package pkg

import (
	"sync"
)

type AtomicIterator struct {
	number int
	mutex  *sync.Mutex
}

func New() *AtomicIterator {
	return &(AtomicIterator{-1, &sync.Mutex{}})
}

func (i *AtomicIterator) Next() int {
	i.mutex.Lock()
	i.number = i.number + 1
	i.mutex.Unlock()
	return i.number
}
