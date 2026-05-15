package utils

import (
	"os"
	"sync"
)

func GetEnvOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func Contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// SymbolMutex is a concurrency-safe per-symbol lock registry.
// The zero value is ready to use.
type SymbolMutex struct {
	m sync.Map // map[string]*sync.Mutex
}

// Get returns the mutex for the given symbol, creating one on first access.
// Safe for concurrent use across goroutines.
func (s *SymbolMutex) Get(symbol string) *sync.Mutex {
	if v, ok := s.m.Load(symbol); ok {
		return v.(*sync.Mutex)
	}
	v, _ := s.m.LoadOrStore(symbol, &sync.Mutex{})
	return v.(*sync.Mutex)
}
