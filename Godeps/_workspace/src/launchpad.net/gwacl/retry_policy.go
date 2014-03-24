// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    forkedHttp "launchpad.net/gwacl/fork/http"
    "net/http"
    "time"
)

// A RetryPolicy object encapsulates all the information needed to define how
// requests should be retried when particular response codes are returned by
// the Windows Azure server.
type RetryPolicy struct {
    // The number of times a request could be retried.  This does not account
    // for the initial request so a value of 3 means that the request might be
    // performed 4 times in total.
    NbRetries int
    // The HTTP status codes of the response for which the request should be
    // retried.
    HttpStatusCodes []int
    // How long the client should wait between retries.
    Delay time.Duration
}

var (
    NoRetryPolicy = RetryPolicy{NbRetries: 0}
)

// isRetryCode returns whether or not the given http status code indicates that
// the request should be retried according to this policy.
func (policy RetryPolicy) isRetryCode(httpStatusCode int) bool {
    for _, code := range policy.HttpStatusCodes {
        if code == httpStatusCode {
            return true
        }
    }
    return false
}

func (policy RetryPolicy) getRetryHelper() *retryHelper {
    return &retryHelper{retriesLeft: policy.NbRetries, policy: &policy}
}

// A retryHelper is a utility object used to enforce a retry policy when
// performing requests.
type retryHelper struct {
    // The maximum number of retries left to perform.
    retriesLeft int
    // The `RetryPolicy` enforced by this retrier.
    policy *RetryPolicy
}

// shouldRetry returns whether or not a request governed by the underlying
// retry policy should be retried.  When it returns 'true', `shouldRetry` also
// waits for the specified amount of time, as dictated by the retry policy.
func (ret *retryHelper) shouldRetry(httpStatusCode int) bool {
    if ret.retriesLeft > 0 && ret.policy.isRetryCode(httpStatusCode) {
        ret.retriesLeft--
        return true
    }
    return false
}

// A retrier is a struct used to repeat a request as governed by a retry
// policy.  retrier is usually created using RetryPolicy.getRetrier().
type retrier struct {
    *retryHelper

    // The client used to perform requests.
    client *http.Client
}

func (ret *retrier) RetryRequest(request *http.Request) (*http.Response, error) {
    for {
        response, err := ret.client.Do(request)
        if err != nil {
            return nil, err
        }
        if !ret.shouldRetry(response.StatusCode) {
            return response, nil
        }
        time.Sleep(ret.policy.Delay)
    }
}

// getRetrier returns a `retrier` object used to enforce the retry policy.
func (policy RetryPolicy) getRetrier(client *http.Client) *retrier {
    helper := policy.getRetryHelper()
    return &retrier{retryHelper: helper, client: client}
}

// A forkedHttpRetrier is a struct used to repeat a request as governed by a
// retry policy.  forkedHttpRetrier is usually created using
// RetryPolicy.getForkedHttpRetrier().  It's the same as the `retrier` struct
// except it deals with the forked version of the http package.
type forkedHttpRetrier struct {
    *retryHelper

    // The client used to perform requests.
    client *forkedHttp.Client
}

func (ret *forkedHttpRetrier) RetryRequest(request *forkedHttp.Request) (*forkedHttp.Response, error) {
    for {
        response, err := ret.client.Do(request)
        if err != nil {
            return nil, err
        }
        if !ret.shouldRetry(response.StatusCode) {
            return response, nil
        }
        time.Sleep(ret.policy.Delay)
    }
}

// getRetrier returns a `retrier` object used to enforce the retry policy.
func (policy RetryPolicy) getForkedHttpRetrier(client *forkedHttp.Client) *forkedHttpRetrier {
    helper := policy.getRetryHelper()
    return &forkedHttpRetrier{retryHelper: helper, client: client}
}
