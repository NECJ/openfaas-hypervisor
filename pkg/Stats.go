package Stats

import (
	"math"
	"sync"
)

type Stats struct {
	vmInitTimeNano       []int64
	vmInitTimeNanoLock   sync.Mutex
	funcExecTimeNano     []int64
	funcExecTimeNanoLock sync.Mutex
}

type StatsSummary struct {
	NumbInitVms         uint16
	VmInitTimeNanoAvg   int64
	VmInitTimeNanoStd   float64
	FuncExecTimeNanoAvg int64
	FuncExecTimeNanoStd float64
}

func NewStats() Stats {
	return Stats{vmInitTimeNanoLock: sync.Mutex{}, funcExecTimeNanoLock: sync.Mutex{}}
}

func (s *Stats) AddVmInitTimeNano(time int64) {
	s.vmInitTimeNanoLock.Lock()
	s.vmInitTimeNano = append(s.vmInitTimeNano, time)
	s.vmInitTimeNanoLock.Unlock()
}

func (s *Stats) AddFuncExecTimeNano(time int64) {
	s.funcExecTimeNanoLock.Lock()
	s.funcExecTimeNano = append(s.funcExecTimeNano, time)
	s.funcExecTimeNanoLock.Unlock()
}

func (s *Stats) GetStatsSummary() StatsSummary {
	s.vmInitTimeNanoLock.Lock()
	s.funcExecTimeNanoLock.Lock()

	vmInitTimeNanoLen, vmInitTimeNanoAvg, vmInitTimeNanoStd := computeLenAvgStd(s.vmInitTimeNano)
	_, funcExecTimeNanoAvg, funcExecTimeNanoStd := computeLenAvgStd(s.funcExecTimeNano)

	var numbFuncExec uint16 = 0
	var funcExecTimeNanoSum int64 = 0
	for _, v := range s.funcExecTimeNano {
		numbFuncExec++
		funcExecTimeNanoSum += v
	}

	s.vmInitTimeNanoLock.Unlock()
	s.funcExecTimeNanoLock.Unlock()

	return StatsSummary{
		NumbInitVms:         vmInitTimeNanoLen,
		VmInitTimeNanoAvg:   vmInitTimeNanoAvg,
		VmInitTimeNanoStd:   vmInitTimeNanoStd,
		FuncExecTimeNanoAvg: funcExecTimeNanoAvg,
		FuncExecTimeNanoStd: funcExecTimeNanoStd,
	}
}

func computeLenAvgStd(data []int64) (uint16, int64, float64) {
	var N uint16 = 0
	var sum int64 = 0
	for _, v := range data {
		N++
		sum += v
	}
	if N == 0 {
		return N, -1, -1
	}
	var avg int64 = sum / int64(N)
	var std float64 = 0
	for _, v := range data {
		std += math.Pow(float64(v)-float64(avg), 2)
	}
	std = math.Sqrt(std / float64(N))
	return N, avg, std
}
