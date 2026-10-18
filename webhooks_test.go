package intosql

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func sign(body string, timestamp int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, body)))

	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyWebhookAcceptsAFreshSignedCallAndNothingElse(t *testing.T) {
	body := []byte(`{"type":"ping"}`)
	header := sign(string(body), 1_700_000_000, "whsec_1")
	at := func(seconds int64) WebhookOption { return WebhookNow(time.Unix(seconds, 0)) }

	cases := []struct {
		name   string
		body   []byte
		header string
		secret string
		opts   []WebhookOption
		want   bool
	}{
		{"signed", body, header, "whsec_1", []WebhookOption{at(1_700_000_100)}, true},
		{"spaces around parts", body, " t=1700000000 , " + strings.Split(header, ",")[1], "whsec_1", []WebhookOption{at(1_700_000_100)}, true},
		{"early clock", body, header, "whsec_1", []WebhookOption{at(1_699_999_800)}, true},
		{"changed body", append(body, ' '), header, "whsec_1", []WebhookOption{at(1_700_000_100)}, false},
		{"other secret", body, header, "whsec_2", []WebhookOption{at(1_700_000_100)}, false},
		{"too old", body, header, "whsec_1", []WebhookOption{at(1_700_001_000)}, false},
		{"from the future", body, header, "whsec_1", []WebhookOption{at(1_699_999_000)}, false},
		{"wider tolerance", body, header, "whsec_1", []WebhookOption{at(1_700_001_000), WebhookTolerance(time.Hour)}, true},
		{"no number", body, "t=abc", "whsec_1", nil, false},
		{"no signature", body, "t=1700000000", "whsec_1", []WebhookOption{at(1_700_000_000)}, false},
		{"no timestamp", body, strings.Split(header, ",")[1], "whsec_1", []WebhookOption{at(1_700_000_000)}, false},
		{"empty", body, "", "whsec_1", nil, false},
		{"cut short", body, header[:len(header)-4], "whsec_1", []WebhookOption{at(1_700_000_000)}, false},
		{"now", body, sign(string(body), time.Now().Unix(), "whsec_1"), "whsec_1", nil, true},
	}

	for _, test := range cases {
		if got := VerifyWebhook(test.body, test.header, test.secret, test.opts...); got != test.want {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func TestParseWebhook(t *testing.T) {
	at := WebhookNow(time.Unix(1_700_000_000, 0))
	parse := func(body string) (*WebhookEvent, error) {
		return ParseWebhook([]byte(body), sign(body, 1_700_000_000, "whsec_1"), "whsec_1", at)
	}

	ping, err := parse(`{"id":"evt_1","type":"ping","created_at":"2026-10-16T14:03:27+00:00","data":{"sent_at":"2026-10-16T14:03:27+00:00"}}`)
	if err != nil {
		t.Fatal(err)
	}

	equal(t, ping.ID, "evt_1")
	equal(t, ping.Type, EventPing)
	equal(t, ping.Ping.SentAt.Unix(), ping.CreatedAt.Unix())

	if ping.Conversion != nil {
		t.Fatal("a ping carried a conversion")
	}

	done, err := parse(`{"id":"evt_2","type":"conversion.done","created_at":"2026-10-16T14:03:27+00:00","data":{"id":"01abc","status":"done","output":{"bytes":2,"url":"https://files.test/o"}}}`)
	if err != nil {
		t.Fatal(err)
	}

	equal(t, done.Conversion.ID, "01abc")
	equal(t, done.Conversion.Output.URL, "https://files.test/o")

	failed, err := parse(`{"id":"evt_3","type":"conversion.failed","created_at":"2026-10-16T14:03:27+00:00","data":{"id":"01abc","status":"failed","error":{"code":"timed_out","message":"Too long.","problems":[]}}}`)
	if err != nil {
		t.Fatal(err)
	}

	equal(t, failed.Conversion.Error.Code, "timed_out")

	later, err := parse(`{"id":"evt_4","type":"conversion.archived","created_at":"2026-10-16T14:03:27+00:00","data":{"x":1}}`)
	if err != nil {
		t.Fatal(err)
	}

	equal(t, string(later.Data), `{"x":1}`)

	if later.Conversion != nil || later.Ping != nil {
		t.Fatal("an unknown event was typed")
	}

	body := `{"id":"evt_1","type":"ping"}`
	if _, err := ParseWebhook([]byte(body), sign(body, 1_700_000_000, "whsec_1"), "whsec_2", at); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("got %v", err)
	}

	equal(t, ErrInvalidSignature.Error(), "intosql: the IntoSQL-Signature does not match this body")

	if _, err := parse(`not json`); err == nil || !strings.Contains(err.Error(), "reading the webhook event") {
		t.Fatalf("got %v", err)
	}

	var syntax *json.UnmarshalTypeError
	if _, err := parse(`{"id":"evt_5","type":"conversion.canceled","created_at":"2026-10-16T14:03:27+00:00","data":"canceled"}`); !errors.As(err, &syntax) {
		t.Fatalf("got %v", err)
	}
}
