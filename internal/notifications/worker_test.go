// SPDX-License-Identifier: AGPL-3.0-only

package notifications

import (
	"context"
	"net"
	"net/textproto"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/outbound"
)

func TestRetryClassification(t *testing.T) {
	for _, code := range []int{400, 421, 450, 499} {
		if !smtpRetry(&textproto.Error{Code: code}) {
			t.Fatalf("SMTP %d should retry", code)
		}
	}
	for _, code := range []int{500, 550} {
		if smtpRetry(&textproto.Error{Code: code}) {
			t.Fatalf("SMTP %d should be permanent", code)
		}
	}
	if got := parseRetryAfter("7200"); got != time.Hour {
		t.Fatalf("Retry-After cap=%s", got)
	}
	if got := parseRetryAfter("120"); got != 2*time.Minute {
		t.Fatalf("Retry-After=%s", got)
	}
}

func TestValidateChannelPrivateOptInAndMetadataBlock(t *testing.T) {
	channel := Channel{Name: "Hook", Kind: "webhook", MinimumSeverity: "warning"}
	secrets := ChannelSecrets{URL: "https://private.test/hook"}
	resolver := fixedResolver{
		"private.test":  {net.ParseIP("10.0.0.10")},
		"metadata.test": {net.ParseIP("169.254.169.254")},
	}
	worker := NewWorker(nil, Config{})
	worker.Policy = outbound.Policy{Resolver: resolver}
	if err := worker.ValidateChannel(context.Background(), channel, secrets); err == nil {
		t.Fatal("private target accepted without opt-in")
	}
	worker.Policy.AllowPrivate = true
	if err := worker.ValidateChannel(context.Background(), channel, secrets); err != nil {
		t.Fatalf("private target rejected with opt-in: %v", err)
	}
	secrets.URL = "https://metadata.test/latest/meta-data"
	if err := worker.ValidateChannel(context.Background(), channel, secrets); err == nil {
		t.Fatal("metadata target accepted with private opt-in")
	}
}

func TestRetrySchedule(t *testing.T) {
	for i, want := range retryDelays {
		got, retry := retryDelay(i+1, true, 0)
		if !retry || got != want {
			t.Fatalf("attempt %d delay=%s retry=%v want=%s", i+1, got, retry, want)
		}
	}
	if _, retry := retryDelay(len(retryDelays)+1, true, 0); retry {
		t.Fatal("retry budget exceeded")
	}
	if got, retry := retryDelay(1, true, 45*time.Minute); !retry || got != 45*time.Minute {
		t.Fatalf("Retry-After delay=%s retry=%v", got, retry)
	}
	if got, retry := retryDelay(1, true, 2*time.Hour); !retry || got != time.Hour {
		t.Fatalf("Retry-After cap=%s retry=%v", got, retry)
	}
}

func TestQueueOverflowIsCountedAndRecorded(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	worker := NewWorker(repo, Config{QueueCapacity: 1})
	worker.queue = make(chan string, 1)
	worker.enqueueDue(context.Background(), []string{"first", "second"})
	worker.enqueueDue(context.Background(), []string{"first"})
	if worker.dropped.Load() != 1 {
		t.Fatalf("dropped=%d", worker.dropped.Load())
	}
	var events int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE type='notification_queue_overflow'`).Scan(&events); err != nil || events != 1 {
		t.Fatalf("overflow events=%d err=%v", events, err)
	}
}

func TestPlainTextPayloadIsBoundedToIncidentSummary(t *testing.T) {
	payload := `{"eventType":"opened","incident":{"title":"Database incident","status":"open","severity":"critical","targetType":"resource","targetId":"db","alertCount":25,"firingAlertCount":2,"openedAt":"2026-01-01T00:00:00Z"}}`
	message := plainText(payload)
	if len(message) > 1000 {
		t.Fatalf("plain text message unexpectedly large: %d", len(message))
	}
	if message == payload {
		t.Fatal("raw JSON was used as the email body")
	}
}

func TestSMTPAddressAndRecipientValidation(t *testing.T) {
	channel := Channel{Name: "Email", Kind: "smtp", MinimumSeverity: "warning", Config: map[string]any{"tlsMode": "starttls"}}
	valid := ChannelSecrets{Host: "smtp.example.test:587", Sender: "Binnacle <sender@example.test>", Recipients: []string{"ops@example.test"}}
	if err := validateChannel(channel, valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Recipients = []string{"ops@example.test\r\nBcc: attacker@example.test"}
	if err := validateChannel(channel, invalid); err == nil {
		t.Fatal("header-injection recipient was accepted")
	}
	invalid = valid
	invalid.Recipients = make([]string, 21)
	if err := validateChannel(channel, invalid); err == nil {
		t.Fatal("more than 20 recipients were accepted")
	}
}
