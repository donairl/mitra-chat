package ws

import (
	"sync"
	"testing"
)

func TestTrySendOverflowClosesOnceAndLaterSendsAreDropped(t *testing.T) {
	c := &Client{userID: "u", rooms: map[string]bool{}, send: make(chan []byte, 1)}

	c.trySend([]byte("fits"))
	c.trySend([]byte("overflows")) // buffer full: closes the connection's queue
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("trySend after overflow panicked: %v", r)
		}
	}()
	c.trySend([]byte("after close"))
	c.trySend([]byte("and again"))

	if got := <-c.send; string(got) != "fits" {
		t.Fatalf("first frame = %q, want the one queued before the overflow", got)
	}
	if _, ok := <-c.send; ok {
		t.Fatal("send channel should be closed and drained")
	}
}

func TestTrySendConcurrentOverflowDoesNotPanic(t *testing.T) {
	c := &Client{userID: "u", rooms: map[string]bool{}, send: make(chan []byte, 1)}
	panics := make(chan any, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- r
				}
			}()
			for j := 0; j < 50; j++ {
				c.trySend([]byte("x"))
			}
		}()
	}
	wg.Wait()
	close(panics)
	for r := range panics {
		t.Fatalf("concurrent trySend panicked: %v", r)
	}
}
