// SPDX-License-Identifier: AGPL-3.0-only

package checks

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

const (
	MinInterval = 10 * time.Second
	MaxInterval = time.Hour
	MinTimeout  = time.Second
	MaxTimeout  = 30 * time.Second
	MaxBodyRead = 64 << 10
)

type FailureCode string

const (
	FailureDNS              FailureCode = "dns"
	FailureTimeout          FailureCode = "timeout"
	FailureConnection       FailureCode = "connection"
	FailureTLS              FailureCode = "tls_handshake"
	FailureUnexpectedStatus FailureCode = "unexpected_status"
	FailureBodyMismatch     FailureCode = "body_mismatch"
	FailureTargetBlocked    FailureCode = "target_blocked"
)

type Check struct {
	ID                string        `json:"id"`
	ResourceID        string        `json:"resourceId"`
	Name              string        `json:"name"`
	URL               string        `json:"url"`
	Method            string        `json:"method"`
	Interval          time.Duration `json:"interval"`
	Timeout           time.Duration `json:"timeout"`
	ExpectedStatusMin int           `json:"expectedStatusMin"`
	ExpectedStatusMax int           `json:"expectedStatusMax"`
	BodySubstring     string        `json:"bodySubstring,omitempty"`
	Required          bool          `json:"required"`
	Enabled           bool          `json:"enabled"`
	CreatedAt         time.Time     `json:"createdAt"`
	UpdatedAt         time.Time     `json:"updatedAt"`
}

func (c Check) Validate() error {
	if strings.TrimSpace(c.ResourceID) == "" || strings.TrimSpace(c.Name) == "" {
		return errors.New("resource and name are required")
	}
	if len(c.Name) > 120 || len(c.URL) > 2048 {
		return errors.New("name or URL is too long")
	}
	target, err := url.Parse(c.URL)
	if err != nil || target.Hostname() == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil {
		return errors.New("target must be an HTTP or HTTPS URL without credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("localhost targets are blocked")
	}
	if c.Method != "GET" && c.Method != "HEAD" {
		return errors.New("method must be GET or HEAD")
	}
	if c.Interval < MinInterval || c.Interval > MaxInterval {
		return errors.New("interval must be between 10 seconds and 1 hour")
	}
	if c.Timeout < MinTimeout || c.Timeout > MaxTimeout {
		return errors.New("timeout must be between 1 and 30 seconds")
	}
	if c.ExpectedStatusMin < 100 || c.ExpectedStatusMax > 599 || c.ExpectedStatusMin > c.ExpectedStatusMax {
		return errors.New("invalid expected status range")
	}
	if len(c.BodySubstring) > 256 {
		return errors.New("body substring exceeds 256 bytes")
	}
	return nil
}

type Result struct {
	CheckID              string        `json:"checkId"`
	Status               string        `json:"status"`
	FailureCode          FailureCode   `json:"failureCode,omitempty"`
	HTTPStatus           int           `json:"httpStatus,omitempty"`
	Latency              time.Duration `json:"-"`
	LatencyMS            int64         `json:"latencyMs,omitempty"`
	CheckedAt            time.Time     `json:"checkedAt"`
	ConsecutiveSuccesses int           `json:"consecutiveSuccesses"`
	ConsecutiveFailures  int           `json:"consecutiveFailures"`
}
