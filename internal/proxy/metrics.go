package proxy

import "sync"

// BackendMetrics holds per-backend connection statistics.
type BackendMetrics struct {
	TotalConns  int64
	ActiveConns int64
	BytesIn     int64
	BytesOut    int64
}

// MetricsStore tracks metrics for all backends of a listener.
type MetricsStore struct {
	mu      sync.Mutex
	metrics map[int]*BackendMetrics
}

// NewMetricsStore creates a new metrics store.
func NewMetricsStore() *MetricsStore {
	return &MetricsStore{
		metrics: make(map[int]*BackendMetrics),
	}
}

func (ms *MetricsStore) getOrCreate(port int) *BackendMetrics {
	m, ok := ms.metrics[port]
	if !ok {
		m = &BackendMetrics{}
		ms.metrics[port] = m
	}
	return m
}

// RecordConnect records a new connection to the backend.
func (ms *MetricsStore) RecordConnect(port int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m := ms.getOrCreate(port)
	m.TotalConns++
	m.ActiveConns++
}

// RecordDisconnect records a disconnection from the backend.
func (ms *MetricsStore) RecordDisconnect(port int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m := ms.getOrCreate(port)
	m.ActiveConns--
}

// AddBytesIn adds to the bytes-in counter for the backend.
func (ms *MetricsStore) AddBytesIn(port int, n int64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m := ms.getOrCreate(port)
	m.BytesIn += n
}

// AddBytesOut adds to the bytes-out counter for the backend.
func (ms *MetricsStore) AddBytesOut(port int, n int64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m := ms.getOrCreate(port)
	m.BytesOut += n
}

// Get returns a copy of the metrics for a port.
func (ms *MetricsStore) Get(port int) BackendMetrics {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m, ok := ms.metrics[port]
	if !ok {
		return BackendMetrics{}
	}
	return *m
}

// All returns a copy of all metrics.
func (ms *MetricsStore) All() map[int]BackendMetrics {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	result := make(map[int]BackendMetrics, len(ms.metrics))
	for port, m := range ms.metrics {
		result[port] = *m
	}
	return result
}

// Remove removes metrics for a port.
func (ms *MetricsStore) Remove(port int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.metrics, port)
}
