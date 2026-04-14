package retryer

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestGetWaitTime(t *testing.T) {

	testCases := []struct {
		retryCount      int
		initialWaitTime float64
		waitTime        time.Duration
	}{
		{0, defaultInitialWaitTime, 1000 * time.Millisecond},
		{1, defaultInitialWaitTime, 2000 * time.Millisecond},
		{2, defaultInitialWaitTime, 4000 * time.Millisecond},
		{3, defaultInitialWaitTime, 8000 * time.Millisecond},
		{4, defaultInitialWaitTime, 16000 * time.Millisecond},
	}

	for _, testCase := range testCases {
		waitTime := getWaitTime(testCase.retryCount, testCase.initialWaitTime)
		assert.Equal(t, testCase.waitTime, waitTime)
	}
}

func TestGetWaitTimeWithCustomInitialWaitTime(t *testing.T) {

	testCases := []struct {
		retryCount      int
		initialWaitTime float64
		waitTime        time.Duration
	}{
		{0, 100, 100 * time.Millisecond},
		{1, 100, 200 * time.Millisecond},
		{2, 100, 400 * time.Millisecond},
		{3, 100, 800 * time.Millisecond},
		{4, 100, 1600 * time.Millisecond},
	}

	for _, testCase := range testCases {
		waitTime := getWaitTime(testCase.retryCount, testCase.initialWaitTime)
		assert.Equal(t, testCase.waitTime, waitTime)
	}
}

func TestRetryerDefaultValues(t *testing.T) {
	r := &Retryer{}
	assert.Equal(t, defaultMaxRetryCount, r.maxRetryCount())
	assert.Equal(t, float64(defaultInitialWaitTime), r.initialWaitTime())
	assert.Equal(t, DefaultClient, r.httpClient())
}

func TestRetryerConfigurableValues(t *testing.T) {
	r := &Retryer{
		MaxRetryCount:   5,
		InitialWaitTime: 500 * time.Millisecond,
		Timeout:         10 * time.Second,
	}
	assert.Equal(t, 5, r.maxRetryCount())
	assert.Equal(t, float64(500), r.initialWaitTime())
	assert.Equal(t, 10*time.Second, r.httpClient().Timeout)
}
