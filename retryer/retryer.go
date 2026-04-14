package retryer

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"time"
)

const defaultMaxRetryCount = 3
const defaultInitialWaitTime = 1000
const defaultTimeout = 40 * time.Second

var DefaultClient = &http.Client{Timeout: defaultTimeout}

var retryStatusCodes = map[int]struct{}{
	429: {},
}

type Retryer struct {
	DoFunc          func(retryer *Retryer, request *Request) (*http.Response, error)
	MaxRetryCount   int
	InitialWaitTime time.Duration
	Timeout         time.Duration
	client          *http.Client
}

func (r *Retryer) maxRetryCount() int {
	if r.MaxRetryCount > 0 {
		return r.MaxRetryCount
	}
	return defaultMaxRetryCount
}

func (r *Retryer) initialWaitTime() float64 {
	if r.InitialWaitTime > 0 {
		return float64(r.InitialWaitTime / time.Millisecond)
	}
	return defaultInitialWaitTime
}

func (r *Retryer) httpClient() *http.Client {
	if r.client != nil {
		return r.client
	}
	if r.Timeout > 0 {
		return &http.Client{Timeout: r.Timeout}
	}
	return DefaultClient
}

func (r *Retryer) Do(request *Request) (*http.Response, error) {
	if r.DoFunc != nil {
		return r.DoFunc(r, request)
	}
	return DoWithExponentialBackoff(r, request)
}

func shouldRetry(statusCode int) bool {
	_, shouldRetry := retryStatusCodes[statusCode]

	if (statusCode >= 500 && statusCode <= 599) || shouldRetry {
		return true
	}
	return false
}

func getWaitTime(retryCount int, initialWaitTime float64) time.Duration {
	waitTime := math.Pow(2, float64(retryCount)) * initialWaitTime
	return time.Duration(waitTime) * time.Millisecond
}

func DoWithExponentialBackoff(retryer *Retryer, request *Request) (*http.Response, error) {

	client := retryer.httpClient()
	maxRetryCount := retryer.maxRetryCount()
	initialWaitTime := retryer.initialWaitTime()

	retryCount := 0
	errMessage := ""
	for {

		if request.body != nil {
			_, err := request.body.Seek(0, 0)
			if err != nil {
				return nil, err
			}
		}
		response, err := client.Do(request.Request)

		if err, ok := err.(net.Error); ok {
			// On error, any Response can be ignored.
			if err.Timeout() {
				logrus.Warn(err)
			} else {
				return nil, err
			}
		} else if shouldRetry(response.StatusCode) {
			// If the returned error is nil, the Response will contain a non-nil
			// Body which the user is expected to close.
			io.Copy(ioutil.Discard, response.Body)
			response.Body.Close()
		} else {
			return response, err
		}

		retryCount++
		if retryCount == maxRetryCount {
			if err != nil {
				errMessage = fmt.Sprintf("last error: %s", err)
			} else {
				errMessage = fmt.Sprintf("status code: %d", response.StatusCode)
			}
			break
		}

		waitDuration := getWaitTime(retryCount-1, initialWaitTime)
		time.Sleep(waitDuration)
	}

	return nil, errors.Errorf("Couldn't get a success response, maximum retry count[%d] is exceeded, %s", maxRetryCount, errMessage)
}
