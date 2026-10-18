package intosql

import (
	"encoding/json"
	"time"
)

// ConversionStatus is where a conversion is. Values added later decode as they are.
type ConversionStatus string

// The statuses a conversion goes through.
const (
	StatusAwaitingUpload ConversionStatus = "awaiting_upload"
	StatusQueued         ConversionStatus = "queued"
	StatusRunning        ConversionStatus = "running"
	StatusDone           ConversionStatus = "done"
	StatusFailed         ConversionStatus = "failed"
	StatusCanceled       ConversionStatus = "canceled"
)

// ConversionFailure is why a conversion failed. Code is stable; new codes may be added within v1.
// It is IntoSQL's (such as "timed_out") or the engine's (such as "encrypted_file").
type ConversionFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// What the engine found that stopped it, such as a spec value out of range.
	Problems []Problem `json:"problems"`
}

// Output is the result of a done conversion, downloadable for ten minutes.
type Output struct {
	Bytes int64  `json:"bytes"`
	URL   string `json:"url"`
}

// UploadTarget is where to PUT the file, right after Create.
type UploadTarget struct {
	Method  string                `json:"method"`
	URL     string                `json:"url"`
	Headers map[string]HeaderList `json:"headers"`
}

// HeaderList is a header's values; the API sends a string or a list of strings.
type HeaderList []string

// UnmarshalJSON reads a string or a list of strings.
func (list *HeaderList) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*list = HeaderList{one}

		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}

	*list = many

	return nil
}

// Conversion is a conversion as the API answers it.
type Conversion struct {
	ID     string           `json:"id"`
	Status ConversionStatus `json:"status"`
	Name   string           `json:"name"`
	// The file's size; 0 until a file sent by URL arrives.
	Bytes  int64      `json:"bytes"`
	Spec   JobSpec    `json:"spec"`
	Report *JobReport `json:"report"`
	// When failed: why.
	Error *ConversionFailure `json:"error"`
	// When done: the result, downloadable for ten minutes.
	Output *Output `json:"output"`
	// Right after Create: where to PUT the file.
	Upload     *UploadTarget `json:"upload"`
	CreatedAt  *time.Time    `json:"created_at"`
	StartedAt  *time.Time    `json:"started_at"`
	FinishedAt *time.Time    `json:"finished_at"`
	// When its files are deleted.
	ExpiresAt time.Time `json:"expires_at"`
}

// CaseShare is what a case sends: the report alone, or the file as well.
type CaseShare string

// What a case can share.
const (
	ShareFile   CaseShare = "file"
	ShareReport CaseShare = "report"
)

// CaseStatus is where a case is: "open", "fixed" or "closed".
type CaseStatus string

// Case is a conversion sent to IntoSQL because it didn't come out right.
type Case struct {
	ID           string     `json:"id"`
	ConversionID *string    `json:"conversion_id"`
	Share        CaseShare  `json:"share"`
	Status       CaseStatus `json:"status"`
	CreatedAt    *time.Time `json:"created_at"`
	// When the case and its file are deleted.
	ExpiresAt time.Time `json:"expires_at"`
}

// Usage is the bytes of conversions the plan includes this month, used and left.
type Usage struct {
	Limit       int64     `json:"limit"`
	Used        int64     `json:"used"`
	Left        int64     `json:"left"`
	PeriodStart time.Time `json:"period_start"`
}

// Page is one page of a list.
type Page[T any] struct {
	Data  []T       `json:"data"`
	Links PageLinks `json:"links"`
	Meta  PageMeta  `json:"meta"`
}

// PageLinks are the addresses of the pages around one.
type PageLinks struct {
	Next *string `json:"next"`
	Prev *string `json:"prev,omitempty"`
}

// PageMeta is a page's size and cursors.
type PageMeta struct {
	PerPage    int     `json:"per_page"`
	NextCursor *string `json:"next_cursor"`
	PrevCursor *string `json:"prev_cursor"`
}

// The webhook event types. Types added later are passed on with Data alone.
const (
	EventConversionDone     = "conversion.done"
	EventConversionFailed   = "conversion.failed"
	EventConversionCanceled = "conversion.canceled"
	EventPing               = "ping"
)

// WebhookEvent is what a webhook call carries. ID is the same on every retry: handle each event
// once.
type WebhookEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	// The event's data as it was sent.
	Data json.RawMessage `json:"data"`
	// The conversion, for conversion.done, conversion.failed and conversion.canceled.
	Conversion *Conversion `json:"-"`
	// The test event's data, for ping.
	Ping *Ping `json:"-"`
}

// Ping is the data of a test event.
type Ping struct {
	SentAt time.Time `json:"sent_at"`
}
