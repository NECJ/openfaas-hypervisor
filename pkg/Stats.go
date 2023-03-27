package pkg

import (
	"math"
	"sort"
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
	VmInitTimeNano95    int64
	VmInitTimeNanoMax   int64
	FuncExecTimeNanoAvg int64
	FuncExecTimeNanoStd float64
	FuncExecTimeNano95  int64
	FuncExecTimeNanoMax int64
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

	vmInitTimeNanoLen, vmInitTimeNanoAvg, vmInitTimeNanoStd, vmInitTimeNano95, vmInitTimeNanoMax := computeLenAvgStd95thMax(s.vmInitTimeNano)
	_, funcExecTimeNanoAvg, funcExecTimeNanoStd, funcExecTimeNano95, funcExecTimeNanoMax := computeLenAvgStd95thMax(s.funcExecTimeNano)

	s.vmInitTimeNanoLock.Unlock()
	s.funcExecTimeNanoLock.Unlock()

	return StatsSummary{
		NumbInitVms:         vmInitTimeNanoLen,
		VmInitTimeNanoAvg:   vmInitTimeNanoAvg,
		VmInitTimeNanoStd:   vmInitTimeNanoStd,
		VmInitTimeNano95:    vmInitTimeNano95,
		VmInitTimeNanoMax:   vmInitTimeNanoMax,
		FuncExecTimeNanoAvg: funcExecTimeNanoAvg,
		FuncExecTimeNanoStd: funcExecTimeNanoStd,
		FuncExecTimeNano95:  funcExecTimeNano95,
		FuncExecTimeNanoMax: funcExecTimeNanoMax,
	}
}

func computeLenAvgStd95thMax(data []int64) (uint16, int64, float64, int64, int64) {
	var N uint16 = 0
	var sum int64 = 0
	for _, v := range data {
		N++
		sum += v
	}
	if N == 0 {
		return N, -1, -1, -1, -1
	}
	var avg int64 = sum / int64(N)
	var std float64 = 0
	for _, v := range data {
		std += math.Pow(float64(v)-float64(avg), 2)
	}
	std = math.Sqrt(std / float64(N))

	sort.Slice(data, func(i, j int) bool {
		return data[i] < data[j]
	})
	percentile95 := data[int(float64(N)*0.95)]

	return N, avg, std, percentile95, data[N-1]
}
