package api

import (
	"sync"
	"testing"
)

func TestBroadcastSubscriberConcurrentAccounting(t *testing.T) {
	s := &BroadcastSubscriber{}
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			s.updateCount(1)
			s.updateCount(-1)
		}()
	}
	workers.Wait()
	if count := s.Count(); count != 0 {
		t.Fatal(count)
	}
}
