// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"errors"
	"strings"
	"time"
)

func Retry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !Transient(err) {
			return err
		}
		d := time.Duration(1<<attempt) * 100 * time.Millisecond
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}
func Transient(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "busy") || strings.Contains(s, "locked") || errors.Is(err, context.DeadlineExceeded)
}
