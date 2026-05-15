package utils_test

import (
	"sync"
	"testing"

	"github.com/aliraad79/Gun/utils"
	"github.com/stretchr/testify/assert"
)

func TestSymbolMutex_ReturnsSameMutexForSameSymbol(t *testing.T) {
	var m utils.SymbolMutex

	a := m.Get("BTC_USDT")
	b := m.Get("BTC_USDT")

	assert.Same(t, a, b, "Get must return the same *sync.Mutex for the same symbol")
}

func TestSymbolMutex_DifferentMutexPerSymbol(t *testing.T) {
	var m utils.SymbolMutex

	btc := m.Get("BTC_USDT")
	eth := m.Get("ETH_USDT")

	assert.NotSame(t, btc, eth, "different symbols must get different mutexes")
}

// Race-checked at -race: many goroutines requesting the same symbol must
// (a) never panic on concurrent map writes and (b) all receive the same lock.
func TestSymbolMutex_ConcurrentSameSymbol(t *testing.T) {
	var m utils.SymbolMutex
	const n = 256

	results := make([]*sync.Mutex, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = m.Get("BTC_USDT")
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		assert.Same(t, results[0], results[i])
	}
}
