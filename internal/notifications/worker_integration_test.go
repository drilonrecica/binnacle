// SPDX-License-Identifier: AGPL-3.0-only

package notifications

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/outbound"
)

type fixedResolver map[string][]net.IP

func (r fixedResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, ip := range r[host] {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			out = append(out, addr)
		}
	}
	return out, nil
}

func testCertificate(t *testing.T, host string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	return cert, pool
}

func testWorkerForAddress(t *testing.T, host, address string, roots *x509.CertPool) *Worker {
	t.Helper()
	return &Worker{Config: Config{DeliveryTimeout: 2 * time.Second}, TLSConfig: &tls.Config{RootCAs: roots}, Policy: outbound.Policy{Resolver: fixedResolver{host: {net.ParseIP("192.0.2.10")}}, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}}
}

func TestWebhookTLSAuthenticationSigningAndRetry(t *testing.T) {
	cert, roots := testCertificate(t, "webhook.test")
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	type received struct{ auth, key, sig, body string }
	got := make(chan received, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- received{r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), r.Header.Get("X-Binnacle-Signature"), string(body)}
		w.WriteHeader(http.StatusNoContent)
	})}
	go server.Serve(listener)
	defer server.Close()
	w := testWorkerForAddress(t, "webhook.test", listener.Addr().String(), roots)
	d := deliveryRow{payload: `{"schemaVersion":1}`, key: "stable-key"}
	retry, code, _ := w.webhook(context.Background(), d, ChannelSecrets{URL: "https://webhook.test/hook", BearerToken: "token", SigningSecret: "sign"})
	if retry || code != "" {
		t.Fatalf("delivery retry=%v code=%s", retry, code)
	}
	select {
	case value := <-got:
		mac := hmac.New(sha256.New, []byte("sign"))
		mac.Write([]byte(d.payload))
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if value.auth != "Bearer token" || value.key != "stable-key" || value.sig != want || value.body != d.payload {
			t.Fatalf("unexpected webhook request: %+v", value)
		}
	case <-time.After(time.Second):
		t.Fatal("webhook was not received")
	}

	badTrust := testWorkerForAddress(t, "webhook.test", listener.Addr().String(), x509.NewCertPool())
	retry, code, _ = badTrust.webhook(context.Background(), d, ChannelSecrets{URL: "https://webhook.test/hook"})
	if !retry || code != "tls_failure" {
		t.Fatalf("TLS classification retry=%v code=%s", retry, code)
	}
}

func TestWebhookRetryAfterAndRedirectPolicy(t *testing.T) {
	cert, roots := testCertificate(t, "webhook.test")
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var mu sync.Mutex
	status := http.StatusTooManyRequests
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		s := status
		mu.Unlock()
		if s == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "7200")
		}
		if s == http.StatusFound {
			w.Header().Set("Location", "https://webhook.test/other")
		}
		w.WriteHeader(s)
	})}
	go server.Serve(listener)
	defer server.Close()
	w := testWorkerForAddress(t, "webhook.test", listener.Addr().String(), roots)
	d := deliveryRow{payload: "{}", key: "key"}
	retry, code, after := w.webhook(context.Background(), d, ChannelSecrets{URL: "https://webhook.test/hook"})
	if !retry || code != "http_429" || after != time.Hour {
		t.Fatalf("429 retry=%v code=%s after=%s", retry, code, after)
	}
	for _, tt := range []struct {
		status int
		retry  bool
	}{{http.StatusBadRequest, false}, {http.StatusRequestTimeout, true}, {http.StatusTooEarly, true}, {http.StatusInternalServerError, true}} {
		mu.Lock()
		status = tt.status
		mu.Unlock()
		retry, code, _ = w.webhook(context.Background(), d, ChannelSecrets{URL: "https://webhook.test/hook"})
		if retry != tt.retry || code != fmt.Sprintf("http_%d", tt.status) {
			t.Fatalf("HTTP %d retry=%v code=%s", tt.status, retry, code)
		}
	}
	mu.Lock()
	status = http.StatusFound
	mu.Unlock()
	retry, code, _ = w.webhook(context.Background(), d, ChannelSecrets{URL: "https://webhook.test/hook"})
	if !retry || code != "network_failure" {
		t.Fatalf("redirect retry=%v code=%s", retry, code)
	}
}

type smtpTestServer struct {
	address  string
	roots    *x509.CertPool
	messages chan string
	close    func()
}

func startSMTPServer(t *testing.T, mode string, mailCode int) smtpTestServer {
	t.Helper()
	cert, roots := testCertificate(t, "smtp.test")
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	messages := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		for {
			conn, err := raw.Accept()
			if err != nil {
				return
			}
			go serveSMTP(conn, mode, mailCode, cert, messages)
		}
	}()
	return smtpTestServer{raw.Addr().String(), roots, messages, func() { close(done); raw.Close() }}
}
func serveSMTP(conn net.Conn, mode string, mailCode int, cert tls.Certificate, messages chan<- string) {
	defer conn.Close()
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	if mode == "implicit" {
		tlsConn := tls.Server(conn, tlsCfg)
		if tlsConn.Handshake() != nil {
			return
		}
		conn = tlsConn
	}
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	reply := func(format string, args ...any) { fmt.Fprintf(writer, format, args...); writer.Flush() }
	reply("220 smtp.test ESMTP\r\n")
	tlsActive := mode == "implicit"
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(command, "EHLO") || strings.HasPrefix(command, "HELO"):
			if !tlsActive && mode == "starttls" {
				reply("250-smtp.test\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
			} else {
				reply("250-smtp.test\r\n250 AUTH PLAIN\r\n")
			}
		case command == "STARTTLS":
			reply("220 Ready to start TLS\r\n")
			tlsConn := tls.Server(conn, tlsCfg)
			if tlsConn.Handshake() != nil {
				return
			}
			conn = tlsConn
			reader = bufio.NewReader(conn)
			writer = bufio.NewWriter(conn)
			tlsActive = true
		case strings.HasPrefix(command, "AUTH PLAIN"):
			reply("235 2.7.0 authenticated\r\n")
		case strings.HasPrefix(command, "MAIL FROM"):
			if mailCode != 0 {
				reply("%d rejected\r\n", mailCode)
			} else {
				reply("250 sender ok\r\n")
			}
		case strings.HasPrefix(command, "RCPT TO"):
			reply("250 recipient ok\r\n")
		case command == "DATA":
			reply("354 end with dot\r\n")
			var body strings.Builder
			for {
				part, e := reader.ReadString('\n')
				if e != nil {
					return
				}
				if part == ".\r\n" {
					break
				}
				body.WriteString(part)
			}
			select {
			case messages <- body.String():
			default:
			}
			reply("250 queued\r\n")
		case command == "QUIT":
			reply("221 bye\r\n")
			return
		default:
			reply("250 ok\r\n")
		}
	}
}

func TestSMTPTLSModesAuthenticationAndMessageID(t *testing.T) {
	for _, mode := range []string{"starttls", "implicit"} {
		t.Run(mode, func(t *testing.T) {
			server := startSMTPServer(t, mode, 0)
			defer server.close()
			w := testWorkerForAddress(t, "smtp.test", server.address, server.roots)
			d := deliveryRow{payload: `{"eventType":"test"}`, key: "smtp-stable-key", config: `{"tlsMode":"` + mode + `"}`}
			retry, code := w.email(context.Background(), d, ChannelSecrets{Host: "smtp.test:465", Username: "user", Password: "password", Sender: "Binnacle <sender@example.test>", Recipients: []string{"ops@example.test"}})
			if retry || code != "" {
				t.Fatalf("retry=%v code=%s", retry, code)
			}
			select {
			case message := <-server.messages:
				if !strings.Contains(message, "Message-ID: <smtp-stable-key@binnacle.local>") || !strings.Contains(message, "Binnacle test notification.") {
					t.Fatalf("unexpected message: %s", message)
				}
			case <-time.After(time.Second):
				t.Fatal("SMTP message not received")
			}
		})
	}
}

func TestSMTPResponseClassification(t *testing.T) {
	for _, tt := range []struct {
		code  int
		retry bool
	}{{450, true}, {550, false}} {
		t.Run(fmt.Sprint(tt.code), func(t *testing.T) {
			server := startSMTPServer(t, "implicit", tt.code)
			defer server.close()
			w := testWorkerForAddress(t, "smtp.test", server.address, server.roots)
			retry, code := w.email(context.Background(), deliveryRow{payload: "{}", key: "key", config: `{"tlsMode":"implicit"}`}, ChannelSecrets{Host: "smtp.test:465", Sender: "sender@example.test", Recipients: []string{"ops@example.test"}})
			if retry != tt.retry || code != fmt.Sprintf("smtp_%d", tt.code) {
				t.Fatalf("retry=%v code=%s", retry, code)
			}
		})
	}
}

func TestSMTPHonorsDeliveryDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		<-time.After(time.Second)
	}()
	w := testWorkerForAddress(t, "smtp.test", listener.Addr().String(), x509.NewCertPool())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	retry, code := w.email(ctx, deliveryRow{payload: "{}", key: "key", config: `{"tlsMode":"implicit"}`}, ChannelSecrets{Host: "smtp.test:465", Sender: "sender@example.test", Recipients: []string{"ops@example.test"}})
	if !retry || code == "" || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("retry=%v code=%s elapsed=%s", retry, code, time.Since(started))
	}
	select {
	case <-accepted:
	default:
		t.Fatal("SMTP connection was not accepted")
	}
}
