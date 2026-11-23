package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Message is one of an answer's errors or notices. Code is what to branch on,
// never Message.
type Message struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PageInfo is a list's paging. Its last page is the one where Page reaches
// TotalPages.
type PageInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Count      int `json:"count"` // the length of the result
	TotalCount int `json:"total_count"`
	TotalPages int `json:"total_pages"`
}

// Answer is a whole envelope, for when its notices or a list's paging matter.
type Answer[T any] struct {
	Result   T
	Messages []Message // notices that aren't failures, such as auth's 3000 for a header it didn't store
	Page     *PageInfo // a list's result_info, or nil
}

// Error is a failed call: its status, and the envelope's errors. A failure
// from in front of the API, such as a proxy's that isn't an envelope, has none.
type Error struct {
	Status     int
	Errors     []Message
	RetryAfter time.Duration // from a 429 or a 503 that says when to try again
}

func (e *Error) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "edgescale: %d %s", e.Status, http.StatusText(e.Status))
	for _, m := range e.Errors {
		fmt.Fprintf(&b, "; %d %s", m.Code, m.Message)
	}
	return b.String()
}

// Has reports whether code is one of the errors, such as 1005 for an invalid
// field.
func (e *Error) Has(code int) bool {
	return slices.ContainsFunc(e.Errors, func(m Message) bool { return m.Code == code })
}

// Decode reads one answer: its result as T on a success, or an *Error. It takes
// a generated call's two results as they come, and closes the body.
func Decode[T any](res *http.Response, err error) (T, error) {
	a, err := Read[T](res, err)
	return a.Result, err
}

// Read is Decode with the whole envelope: the result, its notices, and a
// list's paging.
func Read[T any](res *http.Response, err error) (Answer[T], error) {
	var a Answer[T]
	if err != nil {
		return a, err
	}
	defer res.Body.Close()
	var e struct {
		Errors     []Message       `json:"errors"`
		Messages   []Message       `json:"messages"`
		Result     json.RawMessage `json:"result"`
		ResultInfo *PageInfo       `json:"result_info"`
	}
	bad := json.NewDecoder(res.Body).Decode(&e)
	if res.StatusCode < 200 || res.StatusCode > 299 {
		f := &Error{Status: res.StatusCode}
		if bad == nil {
			f.Errors = e.Errors
		}
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 {
			f.RetryAfter = time.Duration(s) * time.Second
		}
		return a, f
	}
	if errors.Is(bad, io.EOF) {
		return a, nil // a success with no body
	}
	if bad != nil {
		return a, fmt.Errorf("edgescale: a %d answer that isn't an envelope: %w", res.StatusCode, bad)
	}
	if len(e.Result) > 0 && string(e.Result) != "null" {
		if err := json.Unmarshal(e.Result, &a.Result); err != nil {
			return a, fmt.Errorf("edgescale: the result isn't a %T: %w", a.Result, err)
		}
	}
	a.Messages, a.Page = e.Messages, e.ResultInfo
	return a, nil
}
