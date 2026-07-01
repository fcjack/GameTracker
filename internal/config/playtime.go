package config

import (
	"os"
	"strconv"
	"time"
)

const (
	envPlaytimeWorkerCount   = "PLAYTIME_WORKER_COUNT"
	envPlaytimeQueueSize     = "PLAYTIME_QUEUE_SIZE"
	envPlaytimeRetryMax      = "PLAYTIME_RETRY_MAX"
	envPlaytimeRatePerSecond = "PLAYTIME_RATE_PER_SECOND"

	defaultPlaytimeWorkerCount   = 3
	defaultPlaytimeQueueSize     = 256
	defaultPlaytimeRetryMax      = 3
	defaultPlaytimeRatePerSecond = 8
	minPlaytimeWorkerCount       = 1
	maxPlaytimeWorkerCount       = 16
)

// PlaytimeWorkerCount returns the number of background workers that consume playtime events.
func PlaytimeWorkerCount() int {
	return parseIntEnv(os.Getenv(envPlaytimeWorkerCount), defaultPlaytimeWorkerCount, minPlaytimeWorkerCount, maxPlaytimeWorkerCount)
}

// PlaytimeQueueSize is the buffered channel capacity for pending playtime events.
func PlaytimeQueueSize() int {
	return parseIntEnv(os.Getenv(envPlaytimeQueueSize), defaultPlaytimeQueueSize, 16, 4096)
}

// PlaytimeRetryMax is the maximum number of retries for a transient playtime fetch failure.
func PlaytimeRetryMax() int {
	return parseIntEnv(os.Getenv(envPlaytimeRetryMax), defaultPlaytimeRetryMax, 0, 10)
}

// PlaytimeRatePerSecond limits outbound Xbox User Stats API calls across all workers.
func PlaytimeRatePerSecond() int {
	return parseIntEnv(os.Getenv(envPlaytimeRatePerSecond), defaultPlaytimeRatePerSecond, 1, 50)
}

// PlaytimeRetryBackoffMax is the maximum delay between playtime fetch retries.
func PlaytimeRetryBackoffMax() time.Duration {
	return 30 * time.Second
}

func parseIntEnv(value string, defaultValue, minValue, maxValue int) int {
	if value == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	if n < minValue {
		return minValue
	}
	if n > maxValue {
		return maxValue
	}
	return n
}
