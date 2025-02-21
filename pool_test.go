package bytepool

import (
	"sync"
	"testing"
)

func TestBytePool_GetPut(t *testing.T) {
	const capSize = 1024
	const max = 10
	pool := NewBytePool(capSize, max, 1)

	b := pool.Get()
	if len(b) != 0 {
		t.Errorf("expected 0, get %d", len(b))
	}
	if cap(b) != capSize {
		t.Errorf("expected %d, get %d", capSize, cap(b))
	}

	pool.Put(b)

	slices := make([][]byte, max)
	for i := 0; i < int(max); i++ {
		slices[i] = pool.Get()
	}

	for i := 0; i < int(max); i++ {
		pool.Put(slices[i])
	}
}

func TestBytePool_Concurrency(t *testing.T) {
	const capSize = 256
	const max = 20
	pool := NewBytePool(capSize, max, 1)
	var wg sync.WaitGroup
	const routines = 1000

	wg.Add(routines)
	for i := 0; i < routines; i++ {
		go func() {

			b := pool.Get()
			b = append(b, byte(1))
			pool.Put(b)
			wg.Done()
		}()
	}
	wg.Wait()
}
