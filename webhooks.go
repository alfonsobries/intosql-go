package intosql

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WebhookOption changes how a webhook call is checked.
type WebhookOption func(*webhookCheck)

type webhookCheck struct {
	tolerance time.Duration
	now       time.Time
}

// WebhookTolerance sets how far the signature's timestamp may be from now. The default is 300
// seconds.
func WebhookTolerance(tolerance time.Duration) WebhookOption {
	return func(check *webhookCheck) {
		check.tolerance = tolerance
	}
}

// WebhookNow fixes the current time, for tests.
func WebhookNow(now time.Time) WebhookOption {
	return func(check *webhookCheck) {
		check.now = now
	}
}

// VerifyWebhook reports whether a webhook call came from IntoSQL: body is the raw request body,
// header the IntoSQL-Signature header, secret the signing secret from Settings, API keys. It is
// false when the header is malformed, the signature doesn't match, or the timestamp is more than
// the tolerance from now.
func VerifyWebhook(body []byte, header, secret string, options ...WebhookOption) bool {
	check := webhookCheck{tolerance: 300 * time.Second, now: time.Now()}
	for _, option := range options {
		option(&check)
	}

	parts := map[string]string{}
	for _, part := range strings.Split(header, ",") {
		name, value, _ := strings.Cut(strings.TrimSpace(part), "=")
		parts[name] = value
	}

	timestamp, err := strconv.ParseInt(parts["t"], 10, 64)
	signature, signed := parts["v1"]

	if err != nil || !signed {
		return false
	}

	age := check.now.Unix() - timestamp
	if age < 0 {
		age = -age
	}

	if age > int64(check.tolerance/time.Second) {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// ParseWebhook returns the event of a webhook call once its signature checks out, and
// ErrInvalidSignature when it doesn't, so a forged or replayed call never reaches your code.
func ParseWebhook(body []byte, header, secret string, options ...WebhookOption) (*WebhookEvent, error) {
	if !VerifyWebhook(body, header, secret, options...) {
		return nil, ErrInvalidSignature
	}

	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("intosql: reading the webhook event: %w", err)
	}

	var err error

	switch event.Type {
	case EventConversionDone, EventConversionFailed, EventConversionCanceled:
		event.Conversion, err = decode[Conversion](event.Data, nil)
	case EventPing:
		event.Ping, err = decode[Ping](event.Data, nil)
	}

	if err != nil {
		return nil, err
	}

	return &event, nil
}
