// SPDX-License-Identifier: AGPL-3.0-only

//go:build acceptance

package acceptance

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// binaryPath is the binnacle executable every suite starts. TestMain builds
// it from ./cmd/binnacle unless BINNACLE_ACCEPTANCE_BINARY names one, which
// is how the release gate tests the binary it is about to publish.
var binaryPath string

const (
	adminUsername = "admin"
	adminPassword = "acceptance correct horse battery"
	// masterKey is a fixed 32-byte key in hex; the TOTP seed is encrypted
	// with it, so every restart of one data directory must reuse it.
	masterKey      = "4f1c2b8e9d7a6035f2e1c4b7a8d9e0f1123456789abcdef0fedcba9876543210"
	startupTimeout = 45 * time.Second
	stopTimeout    = 15 * time.Second
	requestTimeout = 10 * time.Second
	// maxResponseBytes holds a full-size export (16 MiB) with room to spare.
	maxResponseBytes = 32 << 20
)

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "acceptance:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	if path := os.Getenv("BINNACLE_ACCEPTANCE_BINARY"); path != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return 0, err
		}
		if _, err := os.Stat(absolute); err != nil {
			return 0, fmt.Errorf("BINNACLE_ACCEPTANCE_BINARY: %w", err)
		}
		binaryPath = absolute
		return m.Run(), nil
	}
	dir, err := os.MkdirTemp("", "binnacle-acceptance-bin-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	binaryPath = filepath.Join(dir, "binnacle")
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/binnacle")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	if output, err := build.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("build binnacle (the embedded web build must exist; run make build): %w\n%s", err, output)
	}
	return m.Run(), nil
}

// lockedBuffer collects a child's stderr while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// instance is one running binnacle process.
type instance struct {
	URL    string
	cmd    *exec.Cmd
	stderr *lockedBuffer
	exited chan struct{}
	err    error
}

// baseEnv is the environment of every child: the parent's, minus any
// BINNACLE_ setting a developer has exported, plus a data directory, a free
// loopback port, the bootstrap administrator and the master key.
func baseEnv(dataDir, listen string) map[string]string {
	return map[string]string{
		"BINNACLE_DATA_DIR":       dataDir,
		"BINNACLE_LISTEN_ADDRESS": listen,
		"BINNACLE_ADMIN_USERNAME": adminUsername,
		"BINNACLE_ADMIN_PASSWORD": adminPassword,
		"BINNACLE_MASTER_KEY":     masterKey,
	}
}

func childEnv(settings map[string]string) []string {
	env := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "BINNACLE_") {
			env = append(env, entry)
		}
	}
	for key, value := range settings {
		env = append(env, key+"="+value)
	}
	return env
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

// spawn starts the binary in demo mode, so no Docker socket or host paths
// are needed; demo mode keeps authentication fully on.
func spawn(t *testing.T, settings map[string]string) *instance {
	t.Helper()
	cmd := exec.Command(binaryPath, "--demo", "--demo-containers", "3", "--demo-checks", "1")
	cmd.Env = childEnv(settings)
	stderr := &lockedBuffer{}
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	inst := &instance{URL: "http://" + settings["BINNACLE_LISTEN_ADDRESS"], cmd: cmd, stderr: stderr, exited: make(chan struct{})}
	go func() {
		inst.err = cmd.Wait()
		close(inst.exited)
	}()
	return inst
}

// start launches binnacle and waits until /healthz answers 200. The process
// is stopped when the test ends.
func start(t *testing.T, settings map[string]string) *instance {
	t.Helper()
	inst := spawn(t, settings)
	t.Cleanup(func() { inst.stop(t) })
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-inst.exited:
			t.Fatalf("binnacle exited during startup: %v\n%s", inst.err, inst.stderr.String())
		default:
		}
		resp, err := client.Get(inst.URL + "/healthz")
		if err == nil {
			closeBody(t, resp)
			if resp.StatusCode == http.StatusOK {
				return inst
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("binnacle did not become healthy within %s\n%s", startupTimeout, inst.stderr.String())
	return nil
}

// clientTransports are the transports the suites' clients and test proxies
// use. Their idle connections are closed before binnacle is stopped: a
// connection that was dialled but never sent a request keeps http.Server
// Shutdown waiting past the stop budget (DSC-32), which is not what these
// suites test.
var (
	transportsMu     sync.Mutex
	clientTransports = []*http.Transport{http.DefaultTransport.(*http.Transport)}
)

func trackTransport(transport *http.Transport) *http.Transport {
	transportsMu.Lock()
	defer transportsMu.Unlock()
	clientTransports = append(clientTransports, transport)
	return transport
}

func closeIdleClientConnections() {
	transportsMu.Lock()
	defer transportsMu.Unlock()
	for _, transport := range clientTransports {
		transport.CloseIdleConnections()
	}
}

// stop sends SIGTERM and waits for a clean exit, killing the process if it
// does not stop in time.
func (inst *instance) stop(t *testing.T) {
	t.Helper()
	select {
	case <-inst.exited:
		return
	default:
	}
	closeIdleClientConnections()
	if err := inst.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("signal binnacle: %v", err)
	}
	select {
	case <-inst.exited:
		if inst.err != nil {
			t.Errorf("binnacle did not stop cleanly: %v\n%s", inst.err, inst.stderr.String())
		}
	case <-time.After(stopTimeout):
		if err := inst.cmd.Process.Kill(); err != nil {
			t.Errorf("kill binnacle: %v", err)
		}
		<-inst.exited
		t.Errorf("binnacle did not stop within %s\n%s", stopTimeout, inst.stderr.String())
	}
}

// runToExit starts binnacle with settings that must refuse to start and
// returns its exit error and output.
func runToExit(t *testing.T, settings map[string]string) (string, error) {
	t.Helper()
	inst := spawn(t, settings)
	select {
	case <-inst.exited:
		return inst.stderr.String(), inst.err
	case <-time.After(startupTimeout):
		if err := inst.cmd.Process.Kill(); err != nil {
			t.Errorf("kill binnacle: %v", err)
		}
		<-inst.exited
		t.Fatalf("binnacle kept running; expected it to refuse to start\n%s", inst.stderr.String())
		return "", nil
	}
}

// browser is an HTTP client with a cookie jar that sends the Origin and
// CSRF header a same-origin browser would.
type browser struct {
	t      *testing.T
	base   string
	origin string
	client *http.Client
	header http.Header
}

func newBrowser(t *testing.T, base string) *browser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &browser{t: t, base: base, origin: base, header: http.Header{}, client: &http.Client{Jar: jar, Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type response struct {
	Status int
	Body   []byte
	Header http.Header
}

func (r response) errorCode() string {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(r.Body, &body) != nil {
		return ""
	}
	return body.Error.Code
}

func (b *browser) do(method, path string, body any) response {
	b.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			b.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, reader)
	if err != nil {
		b.t.Fatal(err)
	}
	for key, values := range b.header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead {
		if b.origin != "" {
			req.Header.Set("Origin", b.origin)
		}
		if csrf := b.cookie("binnacle_csrf"); csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
	}
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer closeBody(b.t, resp)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		b.t.Fatal(err)
	}
	return response{Status: resp.StatusCode, Body: raw, Header: resp.Header}
}

func (b *browser) cookie(name string) string {
	b.t.Helper()
	req, err := http.NewRequest(http.MethodGet, b.base+"/", nil)
	if err != nil {
		b.t.Fatal(err)
	}
	for _, cookie := range b.client.Jar.Cookies(req.URL) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Errorf("close response body: %v", err)
	}
}

// expect fails the test unless the response has the wanted status and, when
// code is not empty, the wanted error code.
func expect(t *testing.T, what string, got response, status int, code string) {
	t.Helper()
	if got.Status != status || (code != "" && got.errorCode() != code) {
		t.Fatalf("%s: got %d %s, want %d %s", what, got.Status, strings.TrimSpace(string(got.Body)), status, code)
	}
}

func decode[T any](t *testing.T, got response) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(got.Body, &value); err != nil {
		t.Fatalf("decode %s: %v", got.Body, err)
	}
	return value
}

// authenticator plays an authenticator app: an RFC 6238 implementation that
// is independent of internal/auth, so a shared bug cannot make both sides
// agree. It hands out codes for strictly increasing steps, as a server that
// refuses replays requires, staying inside the one-step drift window.
type authenticator struct {
	seed []byte
	last int64
}

const period = 30

func newAuthenticator(t *testing.T, encoded string) *authenticator {
	t.Helper()
	seed, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encoded))
	if err != nil {
		t.Fatalf("decode TOTP seed %q: %v", encoded, err)
	}
	return &authenticator{seed: seed}
}

func (a *authenticator) codeAt(step int64) string {
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], uint64(step))
	mac := hmac.New(sha1.New, a.seed)
	mac.Write(message[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000)
}

// next returns a code for the earliest step the server will still accept
// that is later than every code handed out before. It waits out a step
// boundary when the chosen step could leave the drift window in flight: only
// the previous step can, when the boundary passes before the server checks.
func (a *authenticator) next() string {
	const margin = 3 * time.Second
	for {
		now := time.Now()
		current := now.Unix() / period
		remaining := time.Unix((current+1)*period, 0).Sub(now)
		step := max(a.last+1, current-1)
		if step > current+1 || (step == current-1 && remaining < margin) {
			time.Sleep(remaining + 200*time.Millisecond)
			continue
		}
		a.last = step
		return a.codeAt(step)
	}
}
