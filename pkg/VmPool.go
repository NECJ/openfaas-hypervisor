package pkg

import (
	"sync/atomic"
)

type VmPool struct {
	new  func() any
	head atomic.Pointer[node]
	tail atomic.Pointer[node]
}

type node struct {
	next atomic.Pointer[node]
	item any
}

func NewPool(new func() any) *VmPool {
	dummyNode := atomic.Pointer[node]{}

	dummyNode.Store(&node{
		next: atomic.Pointer[node]{},
		item: atomic.Pointer[any]{},
	})

	return &VmPool{
		new:  new,
		head: dummyNode,
		tail: dummyNode,
	}
}

func (p *VmPool) Put(item any) {
	newNode := &node{
		next: atomic.Pointer[node]{},
		item: item,
	}

	for true {
		localTail := p.tail.Load()
		if localTail.next.CompareAndSwap(nil, newNode) {
			p.tail.Swap(newNode)
			return
		}
	}
}

func (p *VmPool) Get() any {
	var item any
	for true {
		localHead := p.head.Load()
		localHeadNext := localHead.next.Load()

		if localHeadNext == nil {
			if p.new != nil {
				newItem := p.new()
				return newItem
			} else {
				return nil
			}
		}

		item = localHeadNext.item

		if p.head.CompareAndSwap(localHead, localHeadNext) {
			break
		}
	}
	return item
}
