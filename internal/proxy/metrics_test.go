package proxy

import (
	"sync"
	"testing"
)

func TestMetricsStoreRecordConnect(t *testing.T) {
	ms := NewMetricsStore()
	ms.RecordConnect(3000)
	ms.RecordConnect(3000)

	m := ms.Get(3000)
	if m.TotalConns != 2 {
		t.Errorf("TotalConns = %d, want 2", m.TotalConns)
	}
	if m.ActiveConns != 2 {
		t.Errorf("ActiveConns = %d, want 2", m.ActiveConns)
	}
}

func TestMetricsStoreRecordDisconnect(t *testing.T) {
	ms := NewMetricsStore()
	ms.RecordConnect(3000)
	ms.RecordConnect(3000)
	ms.RecordDisconnect(3000)

	m := ms.Get(3000)
	if m.TotalConns != 2 {
		t.Errorf("TotalConns = %d, want 2", m.TotalConns)
	}
	if m.ActiveConns != 1 {
		t.Errorf("ActiveConns = %d, want 1", m.ActiveConns)
	}
}

func TestMetricsStoreByteCounting(t *testing.T) {
	ms := NewMetricsStore()
	ms.RecordConnect(3000)
	ms.AddBytesIn(3000, 100)
	ms.AddBytesOut(3000, 200)
	ms.AddBytesIn(3000, 50)

	m := ms.Get(3000)
	if m.BytesIn != 150 {
		t.Errorf("BytesIn = %d, want 150", m.BytesIn)
	}
	if m.BytesOut != 200 {
		t.Errorf("BytesOut = %d, want 200", m.BytesOut)
	}
}

func TestMetricsStoreGetUnknown(t *testing.T) {
	ms := NewMetricsStore()
	m := ms.Get(9999)
	if m.TotalConns != 0 || m.ActiveConns != 0 {
		t.Error("unknown port should return zero metrics")
	}
}

func TestMetricsStoreAll(t *testing.T) {
	ms := NewMetricsStore()
	ms.RecordConnect(3000)
	ms.RecordConnect(3001)

	all := ms.All()
	if len(all) != 2 {
		t.Errorf("All() returned %d entries, want 2", len(all))
	}
}

func TestMetricsStoreRemove(t *testing.T) {
	ms := NewMetricsStore()
	ms.RecordConnect(3000)
	ms.Remove(3000)

	all := ms.All()
	if len(all) != 0 {
		t.Errorf("All() returned %d entries after remove, want 0", len(all))
	}
}

func TestMetricsStoreConcurrent(t *testing.T) {
	ms := NewMetricsStore()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ms.RecordConnect(3000)
			ms.AddBytesIn(3000, 10)
			ms.AddBytesOut(3000, 20)
			ms.RecordDisconnect(3000)
		}()
	}
	wg.Wait()

	m := ms.Get(3000)
	if m.TotalConns != 100 {
		t.Errorf("TotalConns = %d, want 100", m.TotalConns)
	}
	if m.ActiveConns != 0 {
		t.Errorf("ActiveConns = %d, want 0", m.ActiveConns)
	}
	if m.BytesIn != 1000 {
		t.Errorf("BytesIn = %d, want 1000", m.BytesIn)
	}
	if m.BytesOut != 2000 {
		t.Errorf("BytesOut = %d, want 2000", m.BytesOut)
	}
}
