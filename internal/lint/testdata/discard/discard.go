// SPDX-License-Identifier: AGPL-3.0-only

// Package discard is the errdiscard fixture. Lines ending in "want" must be
// reported; every other assignment to _ must not.
package discard

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"os"
	"strconv"
	"strings"
)

type custom struct{}

func (*custom) Error() string { return "custom" }

func fails() error              { return errors.New("x") }
func pair() (int, error)        { return 0, nil }
func concrete() *custom         { return nil }
func values() (int, bool)       { return 0, true }
func three() (int, int, error)  { return 0, 0, nil }
func noError() (string, string) { return "", "" }

var _ = fails() // want

func Examples(m map[string]int, v any) {
	_ = fails()                                         // want
	_, _ = pair()                                       // want
	n, _ := strconv.Atoi("1")                           // want
	_ = concrete()                                      // want
	_, _, _ = three()                                   // want
	_ = os.Remove("/nonexistent")                       // want
	_, _ = 1, fails()                                   // want
	_, ok := m["k"]                                     // a bool, not an error
	_, isString := v.(string)                           // a bool, not an error
	_, _ = values()                                     // no error in the tuple
	_, _ = noError()                                    // no error in the tuple
	_ = n                                               // an int
	_, _ = rand.Read(make([]byte, 8))                   // infallible since Go 1.24
	_, _ = hmac.New(sha256.New, nil).Write([]byte("x")) // hash.Hash.Write never fails
	var buf bytes.Buffer
	_, _ = buf.WriteString("x") // bytes.Buffer never fails
	var sb strings.Builder
	_, _ = sb.WriteString("x") // strings.Builder never fails
	_, _ = ok, isString
}
