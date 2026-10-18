package intosql

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Conversions exposes each step of a conversion on its own, for when you want to upload from
// somewhere else, start later, or not wait.
type Conversions struct {
	client *Client
}

// CreateParams open a conversion of a file you upload.
type CreateParams struct {
	Name string `json:"name"`
	// The file's exact size in bytes.
	Bytes int64   `json:"bytes"`
	Spec  JobSpec `json:"spec"`
}

// CreateFromURLParams queue a conversion of the file at a public HTTPS address.
type CreateFromURLParams struct {
	URL  string  `json:"url"`
	Spec JobSpec `json:"spec"`
	// Taken from the download when empty.
	Name string `json:"name,omitempty"`
}

// ListParams narrow a list. Zero values are left out.
type ListParams struct {
	// Conversions per page, from 1 to 100. The API's default is 50.
	Limit int
	// Keep only conversions in this status.
	Status ConversionStatus
}

// CaseParams send a conversion to IntoSQL.
type CaseParams struct {
	// Must be true: the person agrees to send it.
	Consent bool `json:"consent"`
	// ShareFile sends the file as well as the report.
	Share CaseShare `json:"share,omitempty"`
	Email string    `json:"email,omitempty"`
	Note  string    `json:"note,omitempty"`
}

// Create opens a conversion; the answer has the address to upload the file to.
func (c *Conversions) Create(ctx context.Context, params CreateParams) (*Conversion, error) {
	return data[Conversion](c.client.request(ctx, http.MethodPost, "/conversions", params))
}

// CreateFromURL queues a conversion of the file at a public HTTPS address.
func (c *Conversions) CreateFromURL(ctx context.Context, params CreateFromURLParams) (*Conversion, error) {
	return data[Conversion](c.client.request(ctx, http.MethodPost, "/conversions", params))
}

// Upload sends size bytes read from file to the address Create answered with, streamed as they
// are. It is never retried: a storage answer other than 2xx is an *Error with code
// "upload_failed".
func (c *Conversions) Upload(ctx context.Context, conversion *Conversion, file io.Reader, size int64) error {
	if conversion.Upload == nil {
		return fmt.Errorf("intosql: conversion %s has no upload address: open it with Create", conversion.ID)
	}

	body := io.NopCloser(file)
	if size == 0 {
		body = http.NoBody
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPut, conversion.Upload.URL, body)
	if err != nil {
		return err
	}

	// A known length, so storage gets one plain PUT instead of a chunked body.
	request.ContentLength = size

	for name, values := range conversion.Upload.Headers {
		request.Header.Set(name, strings.Join(values, ", "))
	}

	response, err := c.client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &Error{
			Status:  response.StatusCode,
			Code:    "upload_failed",
			Message: fmt.Sprintf("The upload answered %d.", response.StatusCode),
			Fields:  map[string][]string{},
		}
	}

	return nil
}

// Start starts a conversion once its file is uploaded whole.
func (c *Conversions) Start(ctx context.Context, id string) (*Conversion, error) {
	return data[Conversion](c.client.request(ctx, http.MethodPost, "/conversions/"+url.PathEscape(id)+"/start", nil))
}

// Retrieve reads a conversion.
func (c *Conversions) Retrieve(ctx context.Context, id string) (*Conversion, error) {
	return data[Conversion](c.client.request(ctx, http.MethodGet, "/conversions/"+url.PathEscape(id), nil))
}

// List iterates over every conversion, newest first, fetching a page at a time as it goes.
func (c *Conversions) List(ctx context.Context, params ListParams) *ListIterator {
	query := url.Values{}

	if params.Limit != 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	}

	if params.Status != "" {
		query.Set("status", string(params.Status))
	}

	path := "/conversions"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	return &ListIterator{ctx: ctx, client: c.client, next: &path}
}

// Cancel stops a conversion that hasn't ended; canceling it again answers the same.
func (c *Conversions) Cancel(ctx context.Context, id string) (*Conversion, error) {
	return data[Conversion](c.client.request(ctx, http.MethodPost, "/conversions/"+url.PathEscape(id)+"/cancel", nil))
}

// CreateCase sends a conversion that didn't come out right to IntoSQL, so support for it gets
// added.
func (c *Conversions) CreateCase(ctx context.Context, id string, params CaseParams) (*Case, error) {
	return data[Case](c.client.request(ctx, http.MethodPost, "/conversions/"+url.PathEscape(id)+"/cases", params))
}

// Delete deletes a conversion's files now instead of after 24 hours.
func (c *Conversions) Delete(ctx context.Context, id string) error {
	_, err := c.client.request(ctx, http.MethodDelete, "/conversions/"+url.PathEscape(id), nil)

	return err
}

// Wait reads the conversion until it is done. It returns *ConversionFailedError if it fails,
// *ConversionCanceledError if it is canceled, *WaitTimeoutError past the timeout, and the
// context's error when ctx ends.
func (c *Conversions) Wait(ctx context.Context, id string, options WaitOptions) (*Conversion, error) {
	timeout := options.Timeout
	if timeout == 0 {
		timeout = time.Hour
	}

	deadline := c.client.now().Add(timeout)

	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		conversion, err := c.Retrieve(ctx, id)
		if err != nil {
			return nil, err
		}

		switch conversion.Status {
		case StatusDone:
			return conversion, nil
		case StatusFailed:
			return nil, newConversionFailedError(id, conversion.Error)
		case StatusCanceled:
			return nil, &ConversionCanceledError{ID: id}
		}

		if !c.client.now().Before(deadline) {
			return nil, &WaitTimeoutError{ID: id, Status: conversion.Status}
		}

		if err := c.client.sleep(ctx, growing(attempt, 5*time.Second)); err != nil {
			return nil, err
		}
	}
}

// ListIterator walks a list a conversion at a time, fetching the next page when it needs it:
//
//	conversions := client.Conversions.List(ctx, intosql.ListParams{})
//	for conversions.Next() {
//		fmt.Println(conversions.Current().ID)
//	}
//	if err := conversions.Err(); err != nil {
//		return err
//	}
type ListIterator struct {
	ctx     context.Context
	client  *Client
	next    *string
	page    []Conversion
	current *Conversion
	err     error
}

// Next moves to the next conversion. It returns false at the end of the list or on an error.
func (it *ListIterator) Next() bool {
	for len(it.page) == 0 {
		if it.err != nil || it.next == nil {
			it.current = nil

			return false
		}

		page, err := decode[Page[Conversion]](it.client.request(it.ctx, http.MethodGet, *it.next, nil))
		if err != nil {
			it.err = err

			continue
		}

		it.page = page.Data
		it.next = page.Links.Next
	}

	it.current = &it.page[0]
	it.page = it.page[1:]

	return true
}

// Current is the conversion Next moved to.
func (it *ListIterator) Current() *Conversion {
	return it.current
}

// Err is the error that stopped the iteration, if any.
func (it *ListIterator) Err() error {
	return it.err
}
