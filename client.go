package intosql

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Version is this package's version, sent in the User-Agent header.
const Version = "0.1.0"

// DefaultBaseURL is the API's address.
const DefaultBaseURL = "https://intosql.com/api/v1"

// Doer sends an HTTP request. *http.Client is one; pass another to go through a proxy or to test.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// SleepFunc pauses between retries and between reads while waiting. It returns ctx.Err() when ctx
// ends first.
type SleepFunc func(ctx context.Context, duration time.Duration) error

// Option changes how a Client is made.
type Option func(*Client)

// WithBaseURL sets the API's base address. A trailing slash is removed.
func WithBaseURL(baseURL string) Option {
	return func(client *Client) {
		client.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithMaxRetries sets the attempts after the first one for rate limits, a full account, server
// errors and lost connections. 0 turns retries off. The default is 2.
func WithMaxRetries(maxRetries int) Option {
	return func(client *Client) {
		client.maxRetries = max(maxRetries, 0)
	}
}

// WithHTTPClient sets what sends every request, the upload included. The default is a plain
// *http.Client.
func WithHTTPClient(doer Doer) Option {
	return func(client *Client) {
		client.http = doer
	}
}

// WithTimeout limits each attempt of an API request. It doesn't apply to the upload, which takes
// as long as the file needs, nor to waiting. 0 turns it off. The default is 60 seconds.
func WithTimeout(timeout time.Duration) Option {
	return func(client *Client) {
		client.timeout = timeout
	}
}

// WithSleep replaces the pause between retries and between reads while waiting.
func WithSleep(sleep SleepFunc) Option {
	return func(client *Client) {
		client.sleep = sleep
	}
}

// Client is the IntoSQL API: convert spreadsheets, CSV and JSON into SQL on IntoSQL's machines.
// It is safe for concurrent use.
type Client struct {
	// Conversions exposes each step of a conversion on its own.
	Conversions *Conversions

	apiKey     string
	baseURL    string
	baseOrigin string
	maxRetries int
	timeout    time.Duration
	http       Doer
	sleep      SleepFunc
	now        func() time.Time
}

// NewClient makes a client with an API key from Settings, API keys.
func NewClient(apiKey string, options ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("intosql: an API key is required: make one in Settings, API keys")
	}

	client := &Client{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		maxRetries: 2,
		timeout:    60 * time.Second,
		http:       &http.Client{},
		sleep:      sleep,
		now:        time.Now,
	}

	for _, option := range options {
		option(client)
	}

	base, err := url.Parse(client.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("intosql: the base URL %q is not an absolute address", client.baseURL)
	}

	client.baseOrigin = origin(base)
	client.Conversions = &Conversions{client: client}

	return client, nil
}

// WaitOptions are how long Wait, Convert and ConvertURL wait. Cancel the context to stop waiting
// early.
type WaitOptions struct {
	// Give up after this long. The default is one hour.
	Timeout time.Duration
}

// ConvertOptions are the options of Convert and ConvertFile.
type ConvertOptions struct {
	// The file's name. It helps tell the format, and the result is named after it. ConvertFile
	// uses the file's base name when it is empty.
	Name string
	Spec JobSpec
	Wait WaitOptions
}

// ConvertURLOptions are the options of ConvertURL.
type ConvertURLOptions struct {
	Spec JobSpec
	// Taken from the download when empty.
	Name string
	Wait WaitOptions
}

// Convert uploads size bytes read from file, converts them and waits for the result.
func (c *Client) Convert(ctx context.Context, file io.Reader, size int64, options ConvertOptions) (*Conversion, error) {
	opened, err := c.Conversions.Create(ctx, CreateParams{Name: options.Name, Bytes: size, Spec: options.Spec})
	if err != nil {
		return nil, err
	}

	if err := c.Conversions.Upload(ctx, opened, file, size); err != nil {
		return nil, err
	}

	if _, err := c.Conversions.Start(ctx, opened.ID); err != nil {
		return nil, err
	}

	return c.Conversions.Wait(ctx, opened.ID, options.Wait)
}

// ConvertFile uploads the file at path, converts it and waits for the result.
func (c *Client) ConvertFile(ctx context.Context, path string, options ConvertOptions) (*Conversion, error) {
	file, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if options.Name == "" {
		options.Name = filepath.Base(path)
	}

	return c.Convert(ctx, file, info.Size(), options)
}

// ConvertURL converts the file at a public HTTPS address and waits for the result.
func (c *Client) ConvertURL(ctx context.Context, fileURL string, options ConvertURLOptions) (*Conversion, error) {
	queued, err := c.Conversions.CreateFromURL(ctx, CreateFromURLParams{URL: fileURL, Spec: options.Spec, Name: options.Name})
	if err != nil {
		return nil, err
	}

	return c.Conversions.Wait(ctx, queued.ID, options.Wait)
}

// Usage returns the bytes of conversions the plan includes this month, used and left.
func (c *Client) Usage(ctx context.Context) (*Usage, error) {
	return data[Usage](c.request(ctx, http.MethodGet, "/usage", nil))
}

type envelope[T any] struct {
	Data T `json:"data"`
}

// data unwraps the {"data": ...} an endpoint answers with.
func data[T any](body []byte, err error) (*T, error) {
	answer, err := decode[envelope[T]](body, err)
	if err != nil {
		return nil, err
	}

	return &answer.Data, nil
}

// decode reads an answer's JSON.
func decode[T any](body []byte, err error) (*T, error) {
	if err != nil {
		return nil, err
	}

	var answer T
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, fmt.Errorf("intosql: reading the answer: %w", err)
	}

	return &answer, nil
}

// openFile opens a file to upload; a variable so tests can hand back one that can't be read.
var openFile = os.Open

// marshalJSON encodes request bodies; a variable so tests can make it fail.
var marshalJSON = json.Marshal

// Codes worth sending again: waiting fixes them. Never already_started, which means it is on its
// way.
var retryableCodes = map[string]bool{
	"rate_limited":        true,
	"too_many_active":     true,
	"request_in_progress": true,
}

// request sends an API request, retrying what waiting fixes, and returns the answer's body.
func (c *Client) request(ctx context.Context, method, path string, input any) ([]byte, error) {
	address, err := c.url(path)
	if err != nil {
		return nil, err
	}

	var payload []byte
	if input != nil {
		if payload, err = marshalJSON(input); err != nil {
			return nil, fmt.Errorf("intosql: encoding the request: %w", err)
		}
	}

	idempotencyKey := ""
	if method == http.MethodPost {
		idempotencyKey = newUUID()
	}

	for attempt := 0; ; attempt++ {
		status, header, body, err := c.attempt(ctx, method, address, payload, idempotencyKey)
		retryAfter := ""

		if err == nil {
			if status >= 200 && status < 300 {
				return body, nil
			}

			apiError := toError(status, header, body)
			if !retryable(apiError) {
				return nil, apiError
			}

			err = apiError
			retryAfter = header.Get("Retry-After")
		} else if ctx.Err() != nil {
			return nil, err
		}

		if attempt >= c.maxRetries {
			return nil, err
		}

		if err := c.sleep(ctx, backoff(attempt, retryAfter)); err != nil {
			return nil, err
		}
	}
}

// attempt sends a request once, within the client's timeout, and reads the whole answer.
func (c *Client) attempt(ctx context.Context, method, address string, payload []byte, idempotencyKey string) (int, http.Header, []byte, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return 0, nil, nil, err
	}

	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "intosql-go/"+Version)

	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer response.Body.Close()

	answer, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, nil, nil, err
	}

	return response.StatusCode, response.Header, answer, nil
}

// url is a path under the API, or one of its own absolute addresses (pagination links).
func (c *Client) url(path string) (string, error) {
	if !strings.HasPrefix(path, "http") {
		return c.baseURL + path, nil
	}

	target, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("intosql: reading %q: %w", path, err)
	}

	if origin(target) != c.baseOrigin {
		return "", fmt.Errorf("intosql: refusing to send the API key to %s", origin(target))
	}

	return path, nil
}

func origin(address *url.URL) string {
	return strings.ToLower(address.Scheme + "://" + address.Host)
}

func toError(status int, header http.Header, body []byte) *Error {
	var answer struct {
		Error *struct {
			Code      string              `json:"code"`
			Message   string              `json:"message"`
			Fields    map[string][]string `json:"fields"`
			RequestID string              `json:"request_id"`
		} `json:"error"`
	}

	apiError := &Error{
		Status:    status,
		Code:      "http_error",
		Message:   fmt.Sprintf("The API answered %d.", status),
		Fields:    map[string][]string{},
		RequestID: header.Get("X-Request-Id"),
	}

	if json.Unmarshal(body, &answer) != nil || answer.Error == nil {
		return apiError
	}

	if answer.Error.Code != "" {
		apiError.Code = answer.Error.Code
	}

	if answer.Error.Message != "" {
		apiError.Message = answer.Error.Message
	}

	if answer.Error.Fields != nil {
		apiError.Fields = answer.Error.Fields
	}

	if answer.Error.RequestID != "" {
		apiError.RequestID = answer.Error.RequestID
	}

	return apiError
}

func retryable(err *Error) bool {
	return retryableCodes[err.Code] || err.Status == 408 || err.Status == 425 || err.Status >= 500
}

// backoff is Retry-After when the answer has it, otherwise half a second doubling up to 8 seconds.
func backoff(attempt int, retryAfter string) time.Duration {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(retryAfter), 64)
	if err == nil && seconds > 0 && !math.IsInf(seconds, 0) {
		return time.Duration(seconds * float64(time.Second))
	}

	return growing(attempt, 8*time.Second)
}

// growing is half a second doubled on every attempt, up to ceiling.
func growing(attempt int, ceiling time.Duration) time.Duration {
	if attempt >= 5 {
		return ceiling
	}

	return min(500*time.Millisecond<<attempt, ceiling)
}

func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// newUUID is a random (version 4) UUID.
func newUUID() string {
	var id [16]byte
	// crypto/rand.Read doesn't fail on supported platforms.
	_, _ = rand.Read(id[:])
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	text := hex.EncodeToString(id[:])

	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:]
}
