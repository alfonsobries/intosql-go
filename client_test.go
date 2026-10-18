package intosql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

const base = "https://intosql.test/api/v1"

type call struct {
	method string
	url    string
	header http.Header
	body   []byte
	length int64
}

// fake is an API answering each call with the next answer in line.
type fake struct {
	answers []any
	calls   []call
}

func (f *fake) Do(request *http.Request) (*http.Response, error) {
	var body []byte
	if request.Body != nil {
		body, _ = io.ReadAll(request.Body)
	}

	f.calls = append(f.calls, call{request.Method, request.URL.String(), request.Header, body, request.ContentLength})

	if len(f.answers) == 0 {
		return nil, fmt.Errorf("no answer queued for %s", request.URL)
	}

	next := f.answers[0]
	f.answers = f.answers[1:]

	if err, ok := next.(error); ok {
		return nil, err
	}

	return next.(*http.Response), nil
}

// sent is the JSON a call sent.
func (c call) sent(t *testing.T) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(c.body, &body); err != nil {
		t.Fatalf("the body isn't JSON: %s", c.body)
	}

	return body
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
func (failingBody) Close() error             { return nil }

// answer is a response with body as JSON, or as it is when it is a string.
func answer(status int, body any, headers ...string) *http.Response {
	text, ok := body.(string)
	if !ok {
		encoded, _ := json.Marshal(body)
		text = string(encoded)
	}

	header := http.Header{"Content-Type": {"application/json"}}
	for index := 0; index < len(headers); index += 2 {
		header.Set(headers[index], headers[index+1])
	}

	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(text))}
}

func conversion(overrides map[string]any) map[string]any {
	value := map[string]any{
		"id":          "01abc",
		"status":      "awaiting_upload",
		"name":        "ventas.csv",
		"bytes":       3,
		"spec":        NewSpec(SpecOptions{Dialect: DialectPostgreSQL}),
		"report":      nil,
		"error":       nil,
		"output":      nil,
		"upload":      nil,
		"created_at":  nil,
		"started_at":  nil,
		"finished_at": nil,
		"expires_at":  "2026-10-16T00:00:00+00:00",
	}

	for key, override := range overrides {
		value[key] = override
	}

	return map[string]any{"data": value}
}

var withUpload = map[string]any{
	"method":  "PUT",
	"url":     "https://storage.test/put",
	"headers": map[string]any{"Content-Type": "text/csv", "X-Multi": []string{"a", "b"}},
}

type harness struct {
	client *Client
	fake   *fake
	sleeps []time.Duration
}

func api(t *testing.T, answers ...any) *harness {
	t.Helper()

	h := &harness{fake: &fake{answers: answers}}
	client, err := NewClient("key_1",
		WithBaseURL(base+"/"),
		WithHTTPClient(h.fake),
		WithSleep(func(_ context.Context, duration time.Duration) error {
			h.sleeps = append(h.sleeps, duration)

			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	h.client = client

	return h
}

func (h *harness) requests() []string {
	lines := []string{}
	for _, c := range h.fake.calls {
		lines = append(lines, c.method+" "+c.url)
	}

	return lines
}

func equal(t *testing.T, got, want any) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func asError[T error](t *testing.T, err error) T {
	t.Helper()

	var target T
	if !errors.As(err, &target) {
		t.Fatalf("got %v (%T), want %T", err, err, target)
	}

	return target
}

func TestNewClientNeedsAKeyAndAnAbsoluteBaseURL(t *testing.T) {
	if _, err := NewClient(""); err == nil || !strings.Contains(err.Error(), "an API key is required") {
		t.Fatalf("got %v", err)
	}

	for _, address := range []string{"http://[::1", "intosql.test/api"} {
		if _, err := NewClient("k", WithBaseURL(address)); err == nil || !strings.Contains(err.Error(), "not an absolute address") {
			t.Fatalf("%s: got %v", address, err)
		}
	}

	client, err := NewClient("k", WithMaxRetries(-1), WithTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	equal(t, client.baseURL, DefaultBaseURL)
	equal(t, client.maxRetries, 0)
	equal(t, client.timeout, time.Second)

	if client.Conversions == nil {
		t.Fatal("no conversions")
	}
}

func TestConvertUploadsStartsAndWaits(t *testing.T) {
	h := api(t,
		answer(201, conversion(map[string]any{"upload": withUpload})),
		answer(200, ""),
		answer(202, conversion(map[string]any{"status": "queued"})),
		answer(200, conversion(map[string]any{"status": "running"})),
		answer(200, conversion(map[string]any{"status": "done", "output": map[string]any{"bytes": 30, "url": "https://storage.test/out"}})),
	)

	done, err := h.client.Convert(context.Background(), strings.NewReader("a,b"), 3, ConvertOptions{
		Name: "ventas.csv",
		Spec: NewSpec(SpecOptions{Dialect: DialectMySQL, Table: "ventas"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	equal(t, done.Output.URL, "https://storage.test/out")
	equal(t, h.requests(), []string{
		"POST " + base + "/conversions",
		"PUT https://storage.test/put",
		"POST " + base + "/conversions/01abc/start",
		"GET " + base + "/conversions/01abc",
		"GET " + base + "/conversions/01abc",
	})
	equal(t, h.sleeps, []time.Duration{500 * time.Millisecond})

	opened := h.fake.calls[0]
	equal(t, opened.header.Get("Authorization"), "Bearer key_1")
	equal(t, opened.header.Get("Accept"), "application/json")
	equal(t, opened.header.Get("Content-Type"), "application/json")
	equal(t, opened.header.Get("User-Agent"), "intosql-go/"+Version)

	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(opened.header.Get("Idempotency-Key")) {
		t.Fatalf("idempotency key %q", opened.header.Get("Idempotency-Key"))
	}

	body := opened.sent(t)
	equal(t, body["name"], "ventas.csv")
	equal(t, body["bytes"], 3.0)
	output := body["spec"].(map[string]any)["output"].(map[string]any)
	equal(t, output["dialect"], "mysql")
	equal(t, output["table"], "ventas")

	upload := h.fake.calls[1]
	equal(t, string(upload.body), "a,b")
	equal(t, upload.length, int64(3))
	equal(t, upload.header.Get("X-Multi"), "a, b")
	equal(t, upload.header.Get("Content-Type"), "text/csv")
	equal(t, upload.header.Get("Authorization"), "")

	if h.fake.calls[3].header.Get("Idempotency-Key") != "" || h.fake.calls[3].header.Get("Content-Type") != "" {
		t.Fatal("a GET sent an idempotency key or a content type")
	}
}

func TestConvertStopsAtTheFirstStepThatFails(t *testing.T) {
	failure := answer(422, map[string]any{"error": map[string]any{"code": "invalid_spec", "message": "Bad spec."}})

	h := api(t, failure)
	_, err := h.client.Convert(context.Background(), strings.NewReader("a"), 1, ConvertOptions{Name: "a.csv"})
	equal(t, asError[*Error](t, err).Code, "invalid_spec")

	h = api(t, answer(201, conversion(nil)))
	_, err = h.client.Convert(context.Background(), strings.NewReader("a"), 1, ConvertOptions{Name: "a.csv"})
	if err == nil || !strings.Contains(err.Error(), "has no upload address") {
		t.Fatalf("got %v", err)
	}

	h = api(t,
		answer(201, conversion(map[string]any{"upload": withUpload})),
		answer(200, ""),
		answer(409, map[string]any{"error": map[string]any{"code": "already_started", "message": "Started."}}),
	)
	_, err = h.client.Convert(context.Background(), strings.NewReader("a"), 1, ConvertOptions{Name: "a.csv"})
	equal(t, asError[*Error](t, err).Code, "already_started")
}

func TestConvertFileNamesTheUploadAfterTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pedidos.json")
	if err := os.WriteFile(path, []byte(`[{"a":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	h := api(t,
		answer(201, conversion(map[string]any{"upload": withUpload})),
		answer(200, ""),
		answer(202, conversion(map[string]any{"status": "queued"})),
		answer(200, conversion(map[string]any{"status": "done"})),
	)

	if _, err := h.client.ConvertFile(context.Background(), path, ConvertOptions{Spec: NewSpec(SpecOptions{Dialect: DialectSQLite})}); err != nil {
		t.Fatal(err)
	}

	equal(t, h.fake.calls[0].sent(t)["name"], "pedidos.json")
	equal(t, h.fake.calls[0].sent(t)["bytes"], 9.0)
	equal(t, string(h.fake.calls[1].body), `[{"a":1}]`)

	h = api(t,
		answer(201, conversion(map[string]any{"upload": withUpload})),
		answer(200, ""),
		answer(202, conversion(map[string]any{"status": "queued"})),
		answer(200, conversion(map[string]any{"status": "done"})),
	)

	if _, err := h.client.ConvertFile(context.Background(), path, ConvertOptions{Name: "orders.json"}); err != nil {
		t.Fatal(err)
	}

	equal(t, h.fake.calls[0].sent(t)["name"], "orders.json")

	if _, err := h.client.ConvertFile(context.Background(), filepath.Join(t.TempDir(), "missing.csv"), ConvertOptions{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v", err)
	}

	defer func() { openFile = os.Open }()
	openFile = func(name string) (*os.File, error) {
		file, err := os.Open(name)
		if err == nil {
			err = file.Close()
		}

		return file, err
	}

	if _, err := h.client.ConvertFile(context.Background(), path, ConvertOptions{}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("got %v", err)
	}
}

func TestUpload(t *testing.T) {
	h := api(t)
	err := h.client.Conversions.Upload(context.Background(), &Conversion{ID: "01abc"}, strings.NewReader("a"), 1)
	equal(t, err.Error(), "intosql: conversion 01abc has no upload address: open it with Create")

	opened := &Conversion{ID: "01abc", Upload: &UploadTarget{Method: "PUT", URL: "https://storage.test/put"}}
	h = api(t,
		answer(200, ""),
		answer(403, "<Error/>"),
		errors.New("connection refused"),
	)

	if err := h.client.Conversions.Upload(context.Background(), opened, strings.NewReader(""), 0); err != nil {
		t.Fatal(err)
	}

	equal(t, h.fake.calls[0].length, int64(0))
	equal(t, len(h.fake.calls[0].body), 0)

	apiError := asError[*Error](t, h.client.Conversions.Upload(context.Background(), opened, strings.NewReader("ab"), 2))
	equal(t, *apiError, Error{Status: 403, Code: "upload_failed", Message: "The upload answered 403.", Fields: map[string][]string{}})

	if err := h.client.Conversions.Upload(context.Background(), opened, strings.NewReader("ab"), 2); err == nil || err.Error() != "connection refused" {
		t.Fatalf("got %v", err)
	}

	equal(t, len(h.fake.calls), 3)

	broken := &Conversion{ID: "01abc", Upload: &UploadTarget{URL: "http://[::1"}}
	if err := h.client.Conversions.Upload(context.Background(), broken, strings.NewReader("a"), 1); err == nil {
		t.Fatal("no error")
	}
}

func TestConvertURL(t *testing.T) {
	h := api(t,
		answer(202, conversion(map[string]any{"status": "queued", "bytes": 0})),
		answer(200, conversion(map[string]any{"status": "failed", "error": map[string]any{
			"code":     "unsupported_format",
			"message":  "ventas.csv isn't a spreadsheet",
			"problems": []any{map[string]any{"code": "unsupported_format", "severity": "error", "args": map[string]any{}}},
		}})),
	)

	_, err := h.client.ConvertURL(context.Background(), "https://files.test/ventas.csv", ConvertURLOptions{
		Spec: NewSpec(SpecOptions{Dialect: DialectSQLServer, Format: FormatCSV}),
		Name: "ventas.csv",
	})

	failed := asError[*ConversionFailedError](t, err)
	equal(t, failed.ID, "01abc")
	equal(t, failed.Code, "unsupported_format")
	equal(t, failed.Error(), "ventas.csv isn't a spreadsheet")
	equal(t, failed.Problems, []Problem{{Code: "unsupported_format", Severity: "error", Args: map[string]string{}}})
	equal(t, h.fake.calls[0].sent(t)["url"], "https://files.test/ventas.csv")
	equal(t, h.fake.calls[0].sent(t)["name"], "ventas.csv")

	h = api(t, answer(202, conversion(map[string]any{"status": "queued"})), answer(200, conversion(map[string]any{"status": "done"})))
	done, err := h.client.ConvertURL(context.Background(), "https://files.test/a.csv", ConvertURLOptions{Spec: NewSpec(SpecOptions{Dialect: DialectMySQL})})
	if err != nil {
		t.Fatal(err)
	}

	equal(t, done.Status, StatusDone)

	if _, named := h.fake.calls[0].sent(t)["name"]; named {
		t.Fatal("an empty name was sent")
	}

	h = api(t, answer(422, map[string]any{"error": map[string]any{"code": "invalid_url", "message": "No."}}))
	_, err = h.client.ConvertURL(context.Background(), "http://files.test/a.csv", ConvertURLOptions{})
	equal(t, asError[*Error](t, err).Code, "invalid_url")
}

func TestFailedErrorWithoutAReason(t *testing.T) {
	err := newConversionFailedError("x", nil)
	equal(t, *err, ConversionFailedError{ID: "x", Code: "internal_error", Problems: []Problem{}, Message: "Conversion x failed."})

	err = newConversionFailedError("x", &ConversionFailure{Code: "timed_out", Message: "Too long."})
	equal(t, err.Problems, []Problem{})
}

func TestWait(t *testing.T) {
	h := api(t,
		answer(200, conversion(map[string]any{"status": "queued"})),
		answer(200, conversion(map[string]any{"status": "running"})),
		answer(200, conversion(map[string]any{"status": "running"})),
		answer(200, conversion(map[string]any{"status": "running"})),
		answer(200, conversion(map[string]any{"status": "running"})),
		answer(200, conversion(map[string]any{"status": "running"})),
		answer(200, conversion(map[string]any{"status": "canceled"})),
	)

	_, err := h.client.Conversions.Wait(context.Background(), "01abc", WaitOptions{})
	canceled := asError[*ConversionCanceledError](t, err)
	equal(t, canceled.ID, "01abc")
	equal(t, canceled.Error(), "Conversion 01abc was canceled.")
	equal(t, h.sleeps, []time.Duration{
		500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second,
	})

	clock := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	h = api(t,
		answer(200, conversion(map[string]any{"status": "running"})),
		answer(200, conversion(map[string]any{"status": "running"})),
	)
	h.client.now = func() time.Time { return clock }
	h.client.sleep = func(_ context.Context, duration time.Duration) error {
		clock = clock.Add(duration)

		return nil
	}

	_, err = h.client.Conversions.Wait(context.Background(), "01abc", WaitOptions{Timeout: 400 * time.Millisecond})
	timeout := asError[*WaitTimeoutError](t, err)
	equal(t, timeout.Error(), "Conversion 01abc is still running.")
	equal(t, timeout.Status, StatusRunning)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := h.client.Conversions.Wait(ctx, "01abc", WaitOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}

	h = api(t, answer(200, conversion(map[string]any{"status": "running"})))
	h.client.sleep = func(context.Context, time.Duration) error { return context.DeadlineExceeded }

	if _, err := h.client.Conversions.Wait(context.Background(), "01abc", WaitOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}

	h = api(t, answer(404, map[string]any{"error": map[string]any{"code": "not_found", "message": "No."}}))
	_, err = h.client.Conversions.Wait(context.Background(), "01abc", WaitOptions{})
	equal(t, asError[*Error](t, err).Code, "not_found")
}

func TestRetriesWithTheSameIdempotencyKey(t *testing.T) {
	h := api(t,
		answer(429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Slow down."}}, "Retry-After", "2"),
		errors.New("connection refused"),
		answer(200, map[string]any{"data": map[string]any{"limit": 10, "used": 1, "left": 9, "period_start": "2026-10-01T00:00:00+00:00"}}),
	)

	usage, err := h.client.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !usage.PeriodStart.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("period start %v", usage.PeriodStart)
	}

	usage.PeriodStart = time.Time{}
	equal(t, *usage, Usage{Limit: 10, Used: 1, Left: 9})
	equal(t, len(h.fake.calls), 3)
	equal(t, h.sleeps, []time.Duration{2 * time.Second, time.Second})

	h = api(t,
		answer(429, map[string]any{"error": map[string]any{"code": "too_many_active", "message": "Full."}}, "Retry-After", "1.5"),
		answer(409, map[string]any{"error": map[string]any{"code": "request_in_progress", "message": "Wait."}}),
		answer(202, conversion(map[string]any{"status": "queued"})),
	)

	started, err := h.client.Conversions.Start(context.Background(), "01abc")
	if err != nil {
		t.Fatal(err)
	}

	equal(t, started.Status, StatusQueued)
	equal(t, h.sleeps, []time.Duration{1500 * time.Millisecond, time.Second})

	keys := map[string]bool{}
	for _, c := range h.fake.calls {
		keys[c.header.Get("Idempotency-Key")] = true
	}

	equal(t, len(keys), 1)

	for _, status := range []int{408, 425, 502, 503, 504, 500} {
		h = api(t, answer(status, "", "Retry-After", "Inf"), answer(201, conversion(nil)))
		if _, err := h.client.Conversions.Create(context.Background(), CreateParams{Name: "a.csv", Bytes: 1, Spec: NewSpec(SpecOptions{Dialect: DialectOracle})}); err != nil {
			t.Fatalf("%d: %v", status, err)
		}

		equal(t, h.fake.calls[0].header.Get("Idempotency-Key"), h.fake.calls[1].header.Get("Idempotency-Key"))
		equal(t, string(h.fake.calls[0].body), string(h.fake.calls[1].body))
	}

	h = api(t,
		answer(429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Slow down."}}),
	)
	h.client.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	if _, err := h.client.Usage(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestBackoffGrowsToItsCeiling(t *testing.T) {
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second}
	for attempt, duration := range want {
		equal(t, backoff(attempt, ""), duration)
	}

	equal(t, backoff(0, "0"), 500*time.Millisecond)
	equal(t, backoff(0, "-1"), 500*time.Millisecond)
	equal(t, backoff(0, "NaN"), 500*time.Millisecond)
	equal(t, backoff(0, "Wed, 21 Oct 2026 07:28:00 GMT"), 500*time.Millisecond)
	equal(t, backoff(3, " 30 "), 30*time.Second)
	equal(t, growing(200, 5*time.Second), 5*time.Second)
}

func TestSleepStopsWithTheContext(t *testing.T) {
	if err := sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestTypedErrors(t *testing.T) {
	h := api(t, answer(422, map[string]any{"error": map[string]any{
		"code": "invalid_request", "message": "Some fields are missing or invalid.", "fields": map[string]any{"spec": []string{"wrong"}},
	}}))

	_, err := h.client.Conversions.Retrieve(context.Background(), "x")
	equal(t, *asError[*Error](t, err), Error{
		Status: 422, Code: "invalid_request", Message: "Some fields are missing or invalid.", Fields: map[string][]string{"spec": {"wrong"}},
	})
	equal(t, err.Error(), "Some fields are missing or invalid.")

	h = api(t, answer(409, map[string]any{"error": map[string]any{"code": "already_started", "message": "Started.", "request_id": "req_1"}}, "X-Request-Id", "req_header"))
	_, err = h.client.Conversions.Start(context.Background(), "x")
	apiError := asError[*Error](t, err)
	equal(t, apiError.Code, "already_started")
	equal(t, apiError.RequestID, "req_1")
	equal(t, len(h.fake.calls), 1)

	h = api(t, answer(500, "not json"), answer(500, "not json"), answer(500, "not json"))
	_, err = h.client.Conversions.Retrieve(context.Background(), "x")
	equal(t, *asError[*Error](t, err), Error{Status: 500, Code: "http_error", Message: "The API answered 500.", Fields: map[string][]string{}})
	equal(t, len(h.fake.calls), 3)

	h = api(t, answer(502, "bad gateway", "X-Request-Id", "req_2"))
	h.client.maxRetries = 0
	_, err = h.client.Usage(context.Background())
	apiError = asError[*Error](t, err)
	equal(t, apiError.Status, 502)
	equal(t, apiError.RequestID, "req_2")

	h = api(t, answer(404, map[string]any{"message": "Not Found"}), answer(400, map[string]any{"error": map[string]any{}}))
	_, err = h.client.Usage(context.Background())
	equal(t, *asError[*Error](t, err), Error{Status: 404, Code: "http_error", Message: "The API answered 404.", Fields: map[string][]string{}})
	_, err = h.client.Usage(context.Background())
	equal(t, asError[*Error](t, err).Code, "http_error")

	offline := errors.New("connection refused")
	h = api(t, offline, offline, offline)
	if _, err := h.client.Usage(context.Background()); !errors.Is(err, offline) {
		t.Fatalf("got %v", err)
	}

	equal(t, len(h.fake.calls), 3)
}

func TestStopsRetryingWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := api(t, context.Canceled)
	if _, err := h.client.Usage(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}

	equal(t, len(h.fake.calls), 1)
}

func TestBrokenAnswers(t *testing.T) {
	h := api(t, answer(200, "not json"))
	if _, err := h.client.Usage(context.Background()); err == nil || !strings.Contains(err.Error(), "reading the answer") {
		t.Fatalf("got %v", err)
	}

	h = api(t, &http.Response{StatusCode: 200, Header: http.Header{}, Body: failingBody{}})
	h.client.maxRetries = 0
	if _, err := h.client.Usage(context.Background()); err == nil || err.Error() != "connection reset" {
		t.Fatalf("got %v", err)
	}

	defer func() { marshalJSON = json.Marshal }()
	marshalJSON = func(any) ([]byte, error) { return nil, errors.New("unsupported value") }

	if _, err := h.client.Conversions.Create(context.Background(), CreateParams{}); err == nil || !strings.Contains(err.Error(), "encoding the request") {
		t.Fatalf("got %v", err)
	}

	if _, err := h.client.request(context.Background(), "BAD METHOD", "/usage", nil); err == nil || !strings.Contains(err.Error(), "invalid method") {
		t.Fatalf("got %v", err)
	}
}

func TestListFollowsEveryPageAndKeepsTheKeyOnItsOrigin(t *testing.T) {
	h := api(t,
		answer(200, map[string]any{"data": []any{conversion(map[string]any{"id": "a"})["data"]}, "links": map[string]any{"next": base + "/conversions?cursor=2"}}),
		answer(200, map[string]any{"data": []any{}, "links": map[string]any{"next": base + "/conversions?cursor=3"}}),
		answer(200, map[string]any{"data": []any{conversion(map[string]any{"id": "b"})["data"], conversion(map[string]any{"id": "c"})["data"]}, "links": map[string]any{"next": "https://evil.test/steal"}}),
		answer(204, ""),
	)

	seen := []string{}
	conversions := h.client.Conversions.List(context.Background(), ListParams{})

	for conversions.Next() {
		seen = append(seen, conversions.Current().ID)
	}

	equal(t, seen, []string{"a", "b", "c"})

	if err := conversions.Err(); err == nil || err.Error() != "intosql: refusing to send the API key to https://evil.test" {
		t.Fatalf("got %v", err)
	}

	if conversions.Next() || conversions.Current() != nil {
		t.Fatal("the list went on after an error")
	}

	if err := h.client.Conversions.Delete(context.Background(), "a b/c"); err != nil {
		t.Fatal(err)
	}

	equal(t, h.requests(), []string{
		"GET " + base + "/conversions",
		"GET " + base + "/conversions?cursor=2",
		"GET " + base + "/conversions?cursor=3",
		"DELETE " + base + "/conversions/a%20b%2Fc",
	})

	for _, test := range []struct {
		params ListParams
		url    string
	}{
		{ListParams{Limit: 10, Status: StatusDone}, base + "/conversions?limit=10&status=done"},
		{ListParams{Limit: 5}, base + "/conversions?limit=5"},
		{ListParams{Status: StatusFailed}, base + "/conversions?status=failed"},
	} {
		h = api(t, answer(200, map[string]any{"data": []any{conversion(map[string]any{"status": "brand_new"})["data"]}, "links": map[string]any{"next": nil}}))
		statuses := []ConversionStatus{}
		conversions := h.client.Conversions.List(context.Background(), test.params)

		for conversions.Next() {
			statuses = append(statuses, conversions.Current().Status)
		}

		if conversions.Err() != nil || conversions.Next() {
			t.Fatal(conversions.Err())
		}

		equal(t, statuses, []ConversionStatus{"brand_new"})
		equal(t, h.fake.calls[0].url, test.url)
	}

	h = api(t, answer(200, map[string]any{"data": []any{}, "links": map[string]any{"next": "http://[::1"}}))
	conversions = h.client.Conversions.List(context.Background(), ListParams{})

	if conversions.Next() || conversions.Err() == nil {
		t.Fatal("a broken link didn't stop the list")
	}
}

func TestCancelAndCreateCase(t *testing.T) {
	h := api(t,
		answer(200, conversion(map[string]any{"status": "canceled"})),
		answer(201, map[string]any{"data": map[string]any{
			"id": "01case", "conversion_id": "01abc", "share": "file", "status": "open", "created_at": nil, "expires_at": "2027-01-15T00:00:00+00:00",
		}}),
	)

	canceled, err := h.client.Conversions.Cancel(context.Background(), "01abc")
	if err != nil {
		t.Fatal(err)
	}

	equal(t, canceled.Status, StatusCanceled)

	opened, err := h.client.Conversions.CreateCase(context.Background(), "01abc", CaseParams{Consent: true, Share: ShareFile, Note: "Dates come out wrong."})
	if err != nil {
		t.Fatal(err)
	}

	equal(t, opened.ID, "01case")
	equal(t, opened.Share, ShareFile)
	equal(t, *opened.ConversionID, "01abc")
	equal(t, h.requests(), []string{"POST " + base + "/conversions/01abc/cancel", "POST " + base + "/conversions/01abc/cases"})
	equal(t, h.fake.calls[1].sent(t), map[string]any{"consent": true, "share": "file", "note": "Dates come out wrong."})

	if len(h.fake.calls[0].body) != 0 || h.fake.calls[0].header.Get("Idempotency-Key") == "" {
		t.Fatal("cancel sent a body or no idempotency key")
	}
}

func TestDecodesAConversionInFull(t *testing.T) {
	h := api(t, answer(200, conversion(map[string]any{
		"status":     "done",
		"report":     map[string]any{"version": 1, "table": "ventas", "rows": map[string]any{"read": 3, "converted": 2, "rejected": 1}},
		"upload":     map[string]any{"method": "PUT", "url": "u", "headers": map[string]any{"A": "1", "B": []string{"2", "3"}}},
		"created_at": "2026-10-16T14:03:27+00:00",
	})))

	got, err := h.client.Conversions.Retrieve(context.Background(), "01abc")
	if err != nil {
		t.Fatal(err)
	}

	equal(t, got.Report.Rows.Converted, int64(2))
	equal(t, got.Upload.Headers, map[string]HeaderList{"A": {"1"}, "B": {"2", "3"}})
	equal(t, got.CreatedAt.Unix(), int64(1792159407))
	equal(t, got.Spec.Output.Dialect, DialectPostgreSQL)

	var list HeaderList
	if err := json.Unmarshal([]byte(`3`), &list); err == nil {
		t.Fatal("a number decoded as headers")
	}
}
