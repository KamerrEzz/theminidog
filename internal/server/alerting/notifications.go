package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Notifier dispatches alert state transitions to an external sink.
type Notifier interface {
	Notify(ctx context.Context, event NotificationEvent) error
}

// NotificationEvent is the payload sent on a FIRING/RESOLVED transition.
type NotificationEvent struct {
	Event   string    `json:"event"`    // "firing" | "resolved"
	Rule    Rule      `json:"rule"`
	Value   float64   `json:"value"`
	FiredAt time.Time `json:"fired_at"`
}

// WebhookNotifier POSTs NotificationEvent JSON to a single URL.
type WebhookNotifier struct {
	url    string
	client *http.Client
	log    *slog.Logger
}

// NewWebhookNotifier constructs a WebhookNotifier with an internal 5s-timeout
// HTTP client. The per-call context deadline still bounds each request.
func NewWebhookNotifier(url string) *WebhookNotifier {
	return &WebhookNotifier{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
		log:    slog.Default(),
	}
}

// Notify POSTs the event as JSON with a 5s context deadline.
// Non-2xx and transport errors are logged and returned.
func (w *WebhookNotifier) Notify(ctx context.Context, event NotificationEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		w.log.Warn("webhook notify failed", "url", w.url, "err", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.log.Warn("webhook notify non-2xx", "url", w.url, "status", resp.StatusCode)
		return fmt.Errorf("webhook %s returned status %d", w.url, resp.StatusCode)
	}
	return nil
}

// notificationConfigItem is the deserialization shape for a single item in
// the ALERT_NOTIFICATIONS JSON array.
type notificationConfigItem struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// ParseNotifications parses ALERT_NOTIFICATIONS env value.
// Expected format: JSON array of {"type":"webhook","url":"<url>"}.
// Empty or whitespace-only string returns (nil, nil) — notifications disabled.
// Unknown type or malformed JSON returns a descriptive error.
func ParseNotifications(raw string) ([]Notifier, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var items []notificationConfigItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("parse ALERT_NOTIFICATIONS: %w", err)
	}
	notifiers := make([]Notifier, 0, len(items))
	for i, item := range items {
		switch item.Type {
		case "webhook":
			notifiers = append(notifiers, NewWebhookNotifier(item.URL))
		default:
			return nil, fmt.Errorf("notification[%d]: unsupported type %q (only \"webhook\" is supported)", i, item.Type)
		}
	}
	return notifiers, nil
}
