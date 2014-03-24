// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "fmt"
    . "launchpad.net/gocheck"
    forkedHttp "launchpad.net/gwacl/fork/http"
    "net/http"
    "time"
)

type retryPolicySuite struct{}

var _ = Suite(&retryPolicySuite{})

func (*retryPolicySuite) TestNoRetryPolicyDoesNotRetry(c *C) {
    c.Check(NoRetryPolicy.NbRetries, Equals, 0)
}

func (*retryPolicySuite) TestDefaultPolicyIsNoRetryPolicy(c *C) {
    c.Check(NoRetryPolicy, DeepEquals, RetryPolicy{})
}

func (*retryPolicySuite) TestIsRetryCodeChecksStatusCode(c *C) {
    c.Check(
        RetryPolicy{HttpStatusCodes: []int{http.StatusConflict}}.isRetryCode(http.StatusConflict),
        Equals, true)
    c.Check(
        RetryPolicy{HttpStatusCodes: []int{}}.isRetryCode(http.StatusOK),
        Equals, false)
    c.Check(
        RetryPolicy{HttpStatusCodes: []int{http.StatusConflict}}.isRetryCode(http.StatusOK),
        Equals, false)

}

func (*retryPolicySuite) TestGetRetryHelperReturnsHelper(c *C) {
    policy := RetryPolicy{NbRetries: 7, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Minute}
    helper := policy.getRetryHelper()
    c.Check(*helper.policy, DeepEquals, policy)
    c.Check(helper.retriesLeft, Equals, policy.NbRetries)
}

type retryHelperSuite struct{}

var _ = Suite(&retryHelperSuite{})

func (*retryHelperSuite) TestShouldRetryExhaustsRetries(c *C) {
    nbTries := 3
    policy := RetryPolicy{NbRetries: nbTries, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Nanosecond}
    helper := policy.getRetryHelper()
    retries := []bool{}
    for i := 0; i < nbTries+1; i++ {
        retries = append(retries, helper.shouldRetry(http.StatusConflict))
    }
    expectedRetries := []bool{true, true, true, false}
    c.Check(retries, DeepEquals, expectedRetries)
}

func (*retryHelperSuite) TestShouldRetryReturnsFalseIfCodeNotInHttpStatusCodes(c *C) {
    policy := RetryPolicy{NbRetries: 10, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Nanosecond}
    helper := policy.getRetryHelper()
    c.Check(helper.shouldRetry(http.StatusOK), Equals, false)
}

type retrierSuite struct{}

var _ = Suite(&retrierSuite{})

func (*retrierSuite) TestGetRetrier(c *C) {
    client := &http.Client{}
    policy := RetryPolicy{NbRetries: 10, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Nanosecond}
    retrier := policy.getRetrier(client)
    c.Check(*retrier.policy, DeepEquals, policy)
    c.Check(retrier.client, DeepEquals, client)
}

func (*retrierSuite) TestRetryRequest(c *C) {
    nbTries := 3
    transport := &MockingTransport{}
    client := &http.Client{Transport: transport}
    for i := 0; i < nbTries; i++ {
        response := makeHttpResponse(http.StatusConflict, "")
        transport.AddExchange(response, nil)
    }
    response := makeHttpResponse(http.StatusOK, "")
    transport.AddExchange(response, nil)

    policy := RetryPolicy{NbRetries: nbTries, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Nanosecond}
    retrier := policy.getRetrier(client)
    req, err := http.NewRequest("GET", "http://example.com/", nil)
    c.Assert(err, IsNil)

    resp, err := retrier.RetryRequest(req)
    c.Assert(err, IsNil)

    c.Check(resp.StatusCode, Equals, http.StatusOK)
    c.Check(transport.ExchangeCount, Equals, nbTries+1)
}

func (*retrierSuite) TestRetryRequestBailsOutWhenError(c *C) {
    nbTries := 3
    transport := &MockingTransport{}
    client := &http.Client{Transport: transport}
    transport.AddExchange(nil, fmt.Errorf("request error"))

    policy := RetryPolicy{NbRetries: nbTries, HttpStatusCodes: []int{http.StatusConflict}, Delay: time.Nanosecond}
    retrier := policy.getRetrier(client)
    req, err := http.NewRequest("GET", "http://example.com/", nil)
    c.Assert(err, IsNil)

    _, err = retrier.RetryRequest(req)
    c.Check(err, ErrorMatches, ".*request error.*")

    c.Check(transport.ExchangeCount, Equals, 1)
}

type forkedHttpRetrierSuite struct{}

var _ = Suite(&forkedHttpRetrierSuite{})

func (*forkedHttpRetrierSuite) TestGetRetrier(c *C) {
    client := &forkedHttp.Client{}
    policy := RetryPolicy{NbRetries: 10, HttpStatusCodes: []int{forkedHttp.StatusConflict}, Delay: time.Nanosecond}
    retrier := policy.getForkedHttpRetrier(client)
    c.Check(*retrier.policy, DeepEquals, policy)
    c.Check(retrier.client, DeepEquals, client)
}

func (*forkedHttpRetrierSuite) TestRetryRequest(c *C) {
    nbTries := 3
    nbRequests := nbTries + 1
    client := &forkedHttp.Client{}
    httpRequests := make(chan *Request, nbRequests)
    server := makeRecordingHTTPServer(httpRequests, http.StatusConflict, nil, nil)
    defer server.Close()

    policy := RetryPolicy{NbRetries: nbTries, HttpStatusCodes: []int{forkedHttp.StatusConflict}, Delay: time.Nanosecond}
    retrier := policy.getForkedHttpRetrier(client)
    req, err := forkedHttp.NewRequest("GET", server.URL, nil)
    c.Assert(err, IsNil)

    resp, err := retrier.RetryRequest(req)
    c.Assert(err, IsNil)

    c.Check(resp.StatusCode, Equals, forkedHttp.StatusConflict)
    c.Check(len(httpRequests), Equals, nbTries+1)
}
