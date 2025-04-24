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

func GetOrCreateMutex(memory map[string]*sync.Mutex, symbol string) *sync.Mutex {
	_, exists := memory[symbol]
	if exists {
		return memory[symbol]
	} else {
		return &sync.Mutex{}
	}
}
