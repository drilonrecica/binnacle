// SPDX-License-Identifier: AGPL-3.0-only

package notifications

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/outbound"
)

type Config struct {
	MaxConcurrency   int
	QueueCapacity    int
	DeliveryTimeout  time.Duration
	ReminderInterval time.Duration
	AllowPrivate     bool
}

type Worker struct {
	Repo    *Repository
	Config  Config
	Policy  outbound.Policy
	Logger  *slog.Logger
	queue   chan string
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	queued  map[string]struct{}
	queueMu sync.Mutex
	dropped atomic.Int64
	// TLSConfig is nil in production, which uses the system trust store. Tests
	// may provide a private CA; the config is cloned for every destination.
	TLSConfig *tls.Config
}

var retryDelays = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 4 * time.Hour, 12 * time.Hour}

func NewWorker(repo *Repository, c Config) *Worker {
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = 4
	}
	if c.QueueCapacity <= 0 {
		c.QueueCapacity = 1000
	}
	if c.DeliveryTimeout <= 0 {
		c.DeliveryTimeout = 15 * time.Second
	}
	if c.ReminderInterval <= 0 {
		c.ReminderInterval = 2 * time.Hour
	}
	if repo != nil {
		repo.SetReminderInterval(c.ReminderInterval)
	}
	return &Worker{Repo: repo, Config: c, Policy: outbound.Policy{AllowPrivate: c.AllowPrivate}, Logger: slog.Default()}
}
func (w *Worker) ValidateChannel(ctx context.Context, c Channel, s ChannelSecrets) error {
	if err := validateChannel(c, s); err != nil {
		return err
	}
	if c.Kind == "webhook" {
		if _, err := w.Policy.ValidateURL(ctx, s.URL, "https"); err != nil {
			return errors.New("webhook target is blocked")
		}
		return nil
	}
	host, _, err := net.SplitHostPort(s.Host)
	if err != nil {
		return errors.New("SMTP host must include a port")
	}
	if _, err = w.Policy.Resolve(ctx, host); err != nil {
		return errors.New("SMTP target is blocked")
	}
	return nil
}
func (w *Worker) Start(ctx context.Context) error {
	if w.Repo == nil || w.Repo.db == nil {
		return errors.New("notification repository unavailable")
	}
	now := time.Now().UTC()
	if _, err := w.Repo.db.ExecContext(ctx, `UPDATE notification_deliveries SET status='pending',started_at=NULL,next_attempt_at=?,updated_at=? WHERE status='in_progress'`, now.Unix(), now.Unix()); err != nil {
		return err
	}
	if err := w.Repo.Reconcile(ctx, now); err != nil {
		return err
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.queue = make(chan string, w.Config.QueueCapacity)
	w.queued = make(map[string]struct{}, w.Config.QueueCapacity)
	w.wg.Add(1 + w.Config.MaxConcurrency)
	go w.dispatch(ctx)
	for i := 0; i < w.Config.MaxConcurrency; i++ {
		go w.run(ctx)
	}
	return nil
}
func (w *Worker) Stop(context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	return nil
}
func (w *Worker) dispatch(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	maintenance := time.NewTicker(time.Minute)
	defer maintenance.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-maintenance.C:
			now := time.Now().UTC()
			_ = w.Repo.ScheduleReminders(ctx, now, w.Config.ReminderInterval)
			_ = w.Repo.Cleanup(ctx, now)
		case <-ticker.C:
			rows, err := w.Repo.db.QueryContext(ctx, `SELECT d.id FROM notification_deliveries d WHERE d.status='pending' AND d.next_attempt_at<=? AND NOT EXISTS (SELECT 1 FROM notification_deliveries active WHERE active.channel_id=d.channel_id AND active.incident_id=d.incident_id AND active.status='in_progress') ORDER BY d.next_attempt_at LIMIT ?`, time.Now().Unix(), w.Config.QueueCapacity+1)
			if err != nil {
				continue
			}
			var ids []string
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
			w.enqueueDue(ctx, ids)
		}
	}
}
func (w *Worker) enqueueDue(ctx context.Context, ids []string) {
	for _, id := range ids {
		w.queueMu.Lock()
		if _, exists := w.queued[id]; exists {
			w.queueMu.Unlock()
			continue
		}
		if w.queued == nil {
			w.queued = make(map[string]struct{})
		}
		w.queued[id] = struct{}{}
		w.queueMu.Unlock()
		select {
		case w.queue <- id:
		default:
			w.queueMu.Lock()
			delete(w.queued, id)
			w.queueMu.Unlock()
			w.dropped.Add(1)
			w.recordOverflow(ctx)
		}
	}
}
func (w *Worker) recordOverflow(ctx context.Context) {
	now := time.Now().UTC()
	_, _ = w.Repo.db.ExecContext(ctx, `INSERT OR IGNORE INTO events(id,ts,type,severity,summary,correlation_key,source,created_at)VALUES(?,?,?,?,?,?,?,?)`, fmt.Sprintf("notification-overflow-%d", now.Unix()/60), now.UnixMilli(), "notification_queue_overflow", "warning", "Notification queue capacity exceeded", "notifications:overflow", "notifications", now.UnixMilli())
}
func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-w.queue:
			w.deliver(ctx, id)
			w.queueMu.Lock()
			delete(w.queued, id)
			w.queueMu.Unlock()
		}
	}
}

type deliveryRow struct {
	id, channelID, event, payload, key, kind, config, secretRef string
	attempt                                                     int
}

func (w *Worker) deliver(parent context.Context, id string) {
	now := time.Now().UTC()
	res, err := w.Repo.db.ExecContext(parent, `UPDATE notification_deliveries SET status='in_progress',started_at=?,updated_at=? WHERE id=? AND status='pending' AND next_attempt_at<=? AND NOT EXISTS (SELECT 1 FROM notification_deliveries active WHERE active.channel_id=notification_deliveries.channel_id AND active.incident_id=notification_deliveries.incident_id AND active.id!=notification_deliveries.id AND active.status='in_progress')`, now.Unix(), now.Unix(), id, now.Unix())
	if err != nil {
		w.finish(parent, id, false, true, "storage_error", 0)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return
	}
	var d deliveryRow
	err = w.Repo.db.QueryRowContext(parent, `SELECT d.id,d.channel_id,d.event_type,d.payload_json,d.idempotency_key,d.attempt_count,c.kind,c.config_json,c.secret_ref FROM notification_deliveries d JOIN notification_channels c ON c.id=d.channel_id WHERE d.id=? AND c.enabled=1 AND c.deleted_at IS NULL`, id).Scan(&d.id, &d.channelID, &d.event, &d.payload, &d.key, &d.attempt, &d.kind, &d.config, &d.secretRef)
	if errors.Is(err, sql.ErrNoRows) {
		w.finish(parent, id, false, false, "channel_unavailable", 0)
		return
	}
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, w.Config.DeliveryTimeout)
	defer cancel()
	secretBytes, err := w.Repo.secrets.Get(ctx, d.secretRef)
	if err != nil {
		code := "secret_unavailable"
		if errors.Is(err, auth.ErrMasterKeyMissing) {
			code = "master_key_missing"
		}
		w.finish(parent, d.id, false, false, code, 0)
		return
	}
	var secrets ChannelSecrets
	if json.Unmarshal(secretBytes, &secrets) != nil {
		w.finish(parent, d.id, false, false, "invalid_configuration", 0)
		return
	}
	retryAfter := time.Duration(0)
	retryable := false
	code := ""
	switch d.kind {
	case "webhook":
		retryable, code, retryAfter = w.webhook(ctx, d, secrets)
	case "smtp":
		retryable, code = w.email(ctx, d, secrets)
	default:
		code = "invalid_configuration"
	}
	w.finish(parent, d.id, code == "", retryable, code, retryAfter)
}

func (w *Worker) webhook(ctx context.Context, d deliveryRow, s ChannelSecrets) (bool, string, time.Duration) {
	u, err := w.Policy.ValidateURL(ctx, s.URL, "https")
	if err != nil {
		return false, "target_blocked", 0
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSHandshakeTimeout: w.Config.DeliveryTimeout, DialContext: w.Policy.DialContext, TLSClientConfig: w.tlsConfig(u.Hostname())}
	client := &http.Client{Transport: transport, Timeout: w.Config.DeliveryTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects disabled") }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, strings.NewReader(d.payload))
	if err != nil {
		return false, "invalid_configuration", 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", d.key)
	req.Header.Set("User-Agent", "Binnacle-Notifications/1")
	if s.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.BearerToken)
	}
	if s.SigningSecret != "" {
		mac := hmac.New(sha256.New, []byte(s.SigningSecret))
		_, _ = mac.Write([]byte(d.payload))
		req.Header.Set("X-Binnacle-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := client.Do(req)
	if err != nil {
		return true, classifyNetwork(err), 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, "", 0
	}
	retry := resp.StatusCode == 408 || resp.StatusCode == 425 || resp.StatusCode == 429 || resp.StatusCode >= 500
	after := time.Duration(0)
	if retry {
		after = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	return retry, fmt.Sprintf("http_%d", resp.StatusCode), after
}
func parseRetryAfter(v string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && seconds > 0 {
		return min(time.Duration(seconds)*time.Second, time.Hour)
	}
	if at, err := http.ParseTime(v); err == nil {
		return min(max(time.Until(at), time.Duration(0)), time.Hour)
	}
	return 0
}
func classifyNetwork(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns_failure"
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) || strings.Contains(strings.ToLower(err.Error()), "tls") {
		return "tls_failure"
	}
	if errors.Is(err, outbound.ErrBlocked) {
		return "target_blocked"
	}
	return "network_failure"
}

func (w *Worker) email(ctx context.Context, d deliveryRow, s ChannelSecrets) (bool, string) {
	host, port, err := net.SplitHostPort(s.Host)
	if err != nil || host == "" || port == "" || len(s.Recipients) < 1 || len(s.Recipients) > 20 {
		return false, "invalid_configuration"
	}
	if _, err = w.Policy.Resolve(ctx, host); err != nil {
		return false, "target_blocked"
	}
	if _, err = mail.ParseAddress(s.Sender); err != nil {
		return false, "invalid_configuration"
	}
	for _, recipient := range s.Recipients {
		if _, err = mail.ParseAddress(recipient); err != nil {
			return false, "invalid_configuration"
		}
	}
	var cfg map[string]any
	_ = json.Unmarshal([]byte(d.config), &cfg)
	mode, _ := cfg["tlsMode"].(string)
	conn, err := w.Policy.DialContext(ctx, "tcp", s.Host)
	if err != nil {
		return true, classifyNetwork(err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConfig := w.tlsConfig(host)
	var client *smtp.Client
	if mode == "implicit" {
		tlsConn := tls.Client(conn, tlsConfig)
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			return true, "tls_failure"
		}
		client, err = smtp.NewClient(tlsConn, host)
	} else if mode == "starttls" {
		client, err = smtp.NewClient(conn, host)
		if err == nil {
			err = client.StartTLS(tlsConfig)
		}
	} else {
		return false, "invalid_configuration"
	}
	if err != nil {
		return smtpRetry(err), smtpCode(err)
	}
	defer client.Close()
	if s.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return smtpRetry(err), smtpCode(err)
		}
	}
	if err = client.Mail(addressOnly(s.Sender)); err != nil {
		return smtpRetry(err), smtpCode(err)
	}
	for _, to := range s.Recipients {
		if err = client.Rcpt(addressOnly(to)); err != nil {
			return smtpRetry(err), smtpCode(err)
		}
	}
	wc, err := client.Data()
	if err != nil {
		return smtpRetry(err), smtpCode(err)
	}
	subject := "Binnacle incident notification"
	body := plainText(d.payload)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s@binnacle.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", s.Sender, strings.Join(s.Recipients, ", "), subject, d.key, body)
	_, err = io.Copy(wc, strings.NewReader(msg))
	closeErr := wc.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return smtpRetry(err), smtpCode(err)
	}
	if err = client.Quit(); err != nil {
		return smtpRetry(err), smtpCode(err)
	}
	return false, ""
}
func (w *Worker) tlsConfig(serverName string) *tls.Config {
	var cfg *tls.Config
	if w.TLSConfig != nil {
		cfg = w.TLSConfig.Clone()
	} else {
		cfg = &tls.Config{}
	}
	cfg.ServerName = serverName
	if cfg.MinVersion < tls.VersionTLS12 {
		cfg.MinVersion = tls.VersionTLS12
	}
	return cfg
}
func addressOnly(v string) string {
	a, err := mail.ParseAddress(v)
	if err != nil {
		return v
	}
	return a.Address
}
func smtpRetry(err error) bool {
	var e *textproto.Error
	if errors.As(err, &e) {
		return e.Code >= 400 && e.Code < 500
	}
	return true
}
func smtpCode(err error) string {
	var e *textproto.Error
	if errors.As(err, &e) {
		return fmt.Sprintf("smtp_%d", e.Code)
	}
	return classifyNetwork(err)
}
func plainText(payload string) string {
	var v struct {
		EventType string   `json:"eventType"`
		Incident  Incident `json:"incident"`
	}
	if json.Unmarshal([]byte(payload), &v) != nil {
		return "Binnacle test notification."
	}
	if v.EventType == "test" {
		return "Binnacle test notification."
	}
	return fmt.Sprintf("Incident %s\n\n%s\nStatus: %s\nSeverity: %s\nTarget: %s %s\nAlerts: %d (%d firing)\nOpened: %s", v.EventType, v.Incident.Title, v.Incident.Status, v.Incident.Severity, v.Incident.TargetType, v.Incident.TargetID, v.Incident.AlertCount, v.Incident.FiringCount, v.Incident.OpenedAt.Format(time.RFC3339))
}

func (w *Worker) finish(ctx context.Context, id string, success, retryable bool, code string, retryAfter time.Duration) {
	now := time.Now().UTC()
	var attempts int
	if err := w.Repo.db.QueryRowContext(ctx, `SELECT attempt_count+1 FROM notification_deliveries WHERE id=?`, id).Scan(&attempts); err != nil {
		return
	}
	if success {
		_, _ = w.Repo.db.ExecContext(ctx, `UPDATE notification_deliveries SET status='succeeded',attempt_count=?,completed_at=?,next_attempt_at=NULL,failure_code=NULL,updated_at=? WHERE id=?`, attempts, now.Unix(), now.Unix(), id)
		return
	}
	if delay, retry := retryDelay(attempts, retryable, retryAfter); retry {
		_, _ = w.Repo.db.ExecContext(ctx, `UPDATE notification_deliveries SET status='pending',attempt_count=?,started_at=NULL,next_attempt_at=?,failure_code=?,updated_at=? WHERE id=?`, attempts, now.Add(delay).Unix(), code, now.Unix(), id)
		return
	}
	_, _ = w.Repo.db.ExecContext(ctx, `UPDATE notification_deliveries SET status='permanent_failure',attempt_count=?,completed_at=?,next_attempt_at=NULL,failure_code=?,updated_at=? WHERE id=?`, attempts, now.Unix(), code, now.Unix(), id)
}
func retryDelay(attempt int, retryable bool, retryAfter time.Duration) (time.Duration, bool) {
	if !retryable || attempt < 1 || attempt > len(retryDelays) {
		return 0, false
	}
	delay := retryDelays[attempt-1]
	if retryAfter > delay {
		delay = min(retryAfter, time.Hour)
	}
	return delay, true
}
func (w *Worker) Health(ctx context.Context) (Health, error) {
	return w.Repo.Health(ctx, w.dropped.Load())
}
func (w *Worker) HealthSnapshot() (int, int, int64, *time.Time) {
	h, err := w.Health(context.Background())
	if err != nil {
		return 0, 0, w.dropped.Load(), nil
	}
	return h.QueueDepth, h.PermanentFailures, h.DroppedDeliveries, h.LastSuccess
}
