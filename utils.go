package main

import "os"

func getEnvOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func contains(list []string, target string) bool {
    for _, item := range list {
        if item == target {
            return true
        }
    }
    return false
}