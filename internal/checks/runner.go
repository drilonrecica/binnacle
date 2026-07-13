// SPDX-License-Identifier: AGPL-3.0-only

package checks

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/drilonrecica/binnacle/internal/outbound"
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Runner struct {
	AllowPrivate bool
	Resolver     Resolver
	Dialer       *net.Dialer
	DialContext  func(context.Context, string, string) (net.Conn, error)
	Now          func() time.Time
}

func (r *Runner) validateURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := r.policy().ValidateURL(ctx, raw, "http", "https")
	if errors.Is(err, outbound.ErrDNS) {
		return nil, fmt.Errorf("%s: %w", FailureDNS, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%s", FailureTargetBlocked)
	}
	return u, nil
}

func (r *Runner) policy() outbound.Policy {
	return outbound.Policy{AllowPrivate: r.AllowPrivate, Resolver: r.Resolver, Dialer: r.Dialer, Dial: r.DialContext}
}

func (r *Runner) Run(ctx context.Context, check Check) Result {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	started := now()
	result := Result{CheckID: check.ID, Status: "failure", CheckedAt: started.UTC()}
	ctx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()
	if _, err := r.validateURL(ctx, check.URL); err != nil {
		result.FailureCode = classify(err)
		return result
	}
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, TLSHandshakeTimeout: check.Timeout,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return r.policy().DialContext(ctx, network, address)
		},
	}
	client := &http.Client{Transport: transport, Timeout: check.Timeout}
	redirects := 0
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		redirects++
		if redirects > 3 {
			return errors.New("redirect limit exceeded")
		}
		_, err := r.validateURL(req.Context(), req.URL.String())
		return err
	}
	req, err := http.NewRequestWithContext(ctx, check.Method, check.URL, nil)
	if err != nil {
		result.FailureCode = FailureTargetBlocked
		return result
	}
	resp, err := client.Do(req)
	result.Latency = now().Sub(started)
	result.LatencyMS = result.Latency.Milliseconds()
	if err != nil {
		result.FailureCode = classify(err)
		return result
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode < check.ExpectedStatusMin || resp.StatusCode > check.ExpectedStatusMax {
		result.FailureCode = FailureUnexpectedStatus
		return result
	}
	if check.BodySubstring != "" && check.Method != http.MethodHead {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, MaxBodyRead+1))
		if readErr != nil {
			result.FailureCode = FailureConnection
			return result
		}
		if len(body) > MaxBodyRead || !bytes.Contains(body, []byte(check.BodySubstring)) {
			result.FailureCode = FailureBodyMismatch
			return result
		}
	}
	result.Status = "success"
	return result
}

func classify(err error) FailureCode {
	s := err.Error()
	for _, code := range []FailureCode{FailureTargetBlocked, FailureDNS} {
		if strings.Contains(s, string(code)) {
			return code
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ETIMEDOUT) {
		return FailureTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return FailureTimeout
	}
	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) || strings.Contains(strings.ToLower(s), "tls") || strings.Contains(strings.ToLower(s), "certificate") {
		return FailureTLS
	}
	return FailureConnection
}
