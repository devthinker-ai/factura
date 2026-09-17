// Package factura is the Go SDK for embedding Factura's e-invoicing engine.
package factura

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Typed errors vendors branch on (not string-matching).
var (
	ErrUnauthorized = &Error{Code: "unauthorized"}
	ErrQuota        = &Error{Code: "quota_exceeded"}
	ErrPlanLimit    = &Error{Code: "plan_limit"}
	ErrRateLimited  = &Error{Code: "rate_limited"}
)

// Error is a Factura API error with status + machine code.
type Error struct {
	Status int
	Code   string
	Detail string
	Body   []byte
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("factura: %s (%d): %s", e.Code, e.Status, e.Detail)
	}
	return fmt.Sprintf("factura: %s (%d)", e.Code, e.Status)
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// Client talks to a Factura HTTP API with a machine API key.
type Client struct {
	BaseURL    string
	Key        string
	HTTPClient *http.Client
}

// New builds a Client.
func New(baseURL, key string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Key:        key,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Ingest uploads raw invoice bytes (XML/PDF).
type Ingest struct {
	ID           int64           `json:"id"`
	ExternalID   string          `json:"external_id"`
	Status       string          `json:"status"`
	Format       string          `json:"format"`
	Report       json.RawMessage `json:"report"`
	VendorName   string          `json:"vendor_name"`
	InvoiceNumber string         `json:"invoice_number"`
}

// Invoice is a list-row summary.
type Invoice struct {
	ID            int64   `json:"id"`
	ExternalID    string  `json:"external_id"`
	Status        string  `json:"status"`
	Format        string  `json:"format"`
	VendorName    string  `json:"vendor_name"`
	InvoiceNumber string  `json:"invoice_number"`
	Total         float64 `json:"total"`
	Currency      string  `json:"currency"`
}

// Usage is GET /usage for the calling key.
type Usage struct {
	Period            string `json:"period"`
	InvoicesThisMonth int    `json:"invoices_this_month"`
	Keys              []struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		InvoicesThisMonth int    `json:"invoices_this_month"`
		Limit             int    `json:"limit"`
	} `json:"keys"`
}

// ResponseMeta carries content-type for binary Generate responses.
type ResponseMeta struct {
	ContentType string
	Status      int
}

// Ingest POSTs /invoices.
func (c *Client) Ingest(ctx context.Context, filename string, data []byte) (*Ingest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/invoices", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	c.auth(req)
	if filename != "" {
		req.Header.Set("X-Factura-External-ID", filename)
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if err := mapStatus(res.StatusCode, body); err != nil {
		return nil, err
	}
	var out Ingest
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Generate POSTs /invoices/generate?format=xrechnung&download=1 and returns document bytes.
func (c *Client) Generate(ctx context.Context, goblJSON []byte) ([]byte, *ResponseMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/invoices/generate?format=xrechnung&download=1", bytes.NewReader(goblJSON))
	if err != nil {
		return nil, nil, err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	meta := &ResponseMeta{ContentType: res.Header.Get("Content-Type"), Status: res.StatusCode}
	if err := mapStatus(res.StatusCode, body); err != nil {
		return nil, meta, err
	}
	return body, meta, nil
}

// List GETs /invoices with an optional raw query string (without leading ?).
func (c *Client) List(ctx context.Context, query string) ([]Invoice, error) {
	url := c.BaseURL + "/invoices"
	if query != "" {
		url += "?" + strings.TrimPrefix(query, "?")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if err := mapStatus(res.StatusCode, body); err != nil {
		return nil, err
	}
	var wrap struct {
		Rows []Invoice `json:"rows"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, err
	}
	return wrap.Rows, nil
}

// Usage GETs /usage.
func (c *Client) Usage(ctx context.Context) (*Usage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/usage", nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if err := mapStatus(res.StatusCode, body); err != nil {
		return nil, err
	}
	var out Usage
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) auth(req *http.Request) {
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
}

func mapStatus(code int, body []byte) error {
	if code >= 200 && code < 300 {
		return nil
	}
	var parsed struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &parsed)
	err := &Error{Status: code, Code: parsed.Error, Detail: parsed.Detail, Body: body}
	switch code {
	case 401:
		err.Code = "unauthorized"
		return fmt.Errorf("%w", join(ErrUnauthorized, err))
	case 403:
		if parsed.Error == "quota_exceeded" {
			return fmt.Errorf("%w", join(ErrQuota, err))
		}
		err.Code = "plan_limit"
		return fmt.Errorf("%w", join(ErrPlanLimit, err))
	case 429:
		err.Code = "rate_limited"
		return fmt.Errorf("%w", join(ErrRateLimited, err))
	default:
		if err.Code == "" {
			err.Code = "invalid"
		}
		return err
	}
}

func join(sentinel, concrete *Error) error {
	concrete.Code = sentinel.Code
	return concrete
}
