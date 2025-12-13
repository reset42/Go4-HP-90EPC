package model

import "sync"

type Measurement struct {
	Value    *float64 `json:"value"`
	ValueStr string   `json:"value_str"`
	Unit     string   `json:"unit"`
	Mode     string   `json:"mode"`
	Auto     bool     `json:"auto"`
	Hold     bool     `json:"hold"`
	Rel      bool     `json:"rel"`
	LowBatt  bool     `json:"low_batt"`
	RawHex   string   `json:"raw"`
}

// LatestBuffer: threadsicherer Puffer für die letzte Messung
type LatestBuffer struct {
	mu     sync.RWMutex
	latest *Measurement
}

func (b *LatestBuffer) Set(m *Measurement) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.latest = m
}

func (b *LatestBuffer) Get() *Measurement {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.latest
}

