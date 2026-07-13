// SPDX-License-Identifier: AGPL-3.0-only

package notifications

import "time"

type Incident struct {
	ID          string        `json:"id"`
	GroupKey    string        `json:"groupKey"`
	Status      string        `json:"status"`
	Severity    string        `json:"severity"`
	TargetType  string        `json:"targetType"`
	TargetID    string        `json:"targetId"`
	Title       string        `json:"title"`
	AlertCount  int           `json:"alertCount"`
	FiringCount int           `json:"firingAlertCount"`
	OpenedAt    time.Time     `json:"openedAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	ResolvedAt  *time.Time    `json:"resolvedAt,omitempty"`
	Version     int           `json:"version"`
	Alerts      []MemberAlert `json:"alerts,omitempty"`
	Deliveries  []Delivery    `json:"deliveries,omitempty"`
}

type MemberAlert struct {
	ID         string     `json:"id"`
	Family     string     `json:"family"`
	Severity   string     `json:"severity"`
	Status     string     `json:"status"`
	Message    string     `json:"message"`
	StartedAt  time.Time  `json:"startedAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

type Channel struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Kind             string         `json:"kind"`
	Enabled          bool           `json:"enabled"`
	MinimumSeverity  string         `json:"minimumSeverity"`
	NotifyResolved   bool           `json:"notifyResolved"`
	Config           map[string]any `json:"config"`
	SecretConfigured bool           `json:"secretConfigured"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type ChannelSecrets struct {
	URL           string   `json:"url,omitempty"`
	BearerToken   string   `json:"bearerToken,omitempty"`
	SigningSecret string   `json:"signingSecret,omitempty"`
	Host          string   `json:"host,omitempty"`
	Username      string   `json:"username,omitempty"`
	Password      string   `json:"password,omitempty"`
	Sender        string   `json:"sender,omitempty"`
	Recipients    []string `json:"recipients,omitempty"`
}
type SecretPatch struct {
	URL           *string
	BearerToken   *string
	SigningSecret *string
	Host          *string
	Username      *string
	Password      *string
	Sender        *string
	Recipients    *[]string
}

type Delivery struct {
	ID             string     `json:"id"`
	ChannelID      string     `json:"channelId"`
	IncidentID     string     `json:"incidentId,omitempty"`
	EventType      string     `json:"eventType"`
	IdempotencyKey string     `json:"idempotencyKey"`
	Status         string     `json:"status"`
	AttemptCount   int        `json:"attemptCount"`
	NextAttemptAt  *time.Time `json:"nextAttemptAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	FailureCode    string     `json:"failureCode,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type Health struct {
	QueueDepth        int        `json:"queueDepth"`
	PermanentFailures int        `json:"permanentFailures"`
	DroppedDeliveries int64      `json:"droppedDeliveries"`
	LastSuccess       *time.Time `json:"lastSuccessfulDelivery,omitempty"`
}
