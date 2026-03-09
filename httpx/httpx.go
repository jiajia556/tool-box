package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	hc *http.Client
}

type Option func(*requestOptions)

type requestOptions struct {
	header http.Header
	query  url.Values

	jsonBody any
	rawBody  io.Reader
	form     url.Values

	expectJSON bool
}

type ClientOption func(*Client)

func New(opts ...ClientOption) *Client {
	c := &Client{
		hc: &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		if hc != nil {
			c.hc = hc
		}
	}
}

func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		if d > 0 {
			c.hc.Timeout = d
		}
	}
}

// ---------------- Options ----------------

func Header(k, v string) Option {
	return func(o *requestOptions) { o.header.Add(k, v) }
}

func Headers(h map[string]string) Option {
	return func(o *requestOptions) {
		for k, v := range h {
			o.header.Set(k, v)
		}
	}
}

func Query(k, v string) Option {
	return func(o *requestOptions) { o.query.Add(k, v) }
}

func Queries(q map[string]string) Option {
	return func(o *requestOptions) {
		for k, v := range q {
			o.query.Set(k, v)
		}
	}
}

func JSONBody(v any) Option {
	return func(o *requestOptions) {
		o.jsonBody = v
		o.expectJSON = true
	}
}

func Form(v url.Values) Option {
	return func(o *requestOptions) { o.form = v }
}

func Body(r io.Reader) Option {
	return func(o *requestOptions) { o.rawBody = r }
}

func ExpectJSON() Option {
	return func(o *requestOptions) { o.expectJSON = true }
}

// ---------------- Default one-liners ----------------

var Default = New()

func Get(ctx context.Context, rawURL string, opts ...Option) ([]byte, error) {
	return Default.Do(ctx, http.MethodGet, rawURL, opts...)
}
func Post(ctx context.Context, rawURL string, opts ...Option) ([]byte, error) {
	return Default.Do(ctx, http.MethodPost, rawURL, opts...)
}

func GetJSON[T any](ctx context.Context, rawURL string, opts ...Option) (T, error) {
	return DoJSON[T](ctx, Default, http.MethodGet, rawURL, opts...)
}
func PostJSON[T any](ctx context.Context, rawURL string, opts ...Option) (T, error) {
	return DoJSON[T](ctx, Default, http.MethodPost, rawURL, opts...)
}
func PutJSON[T any](ctx context.Context, rawURL string, opts ...Option) (T, error) {
	return DoJSON[T](ctx, Default, http.MethodPut, rawURL, opts...)
}
func PatchJSON[T any](ctx context.Context, rawURL string, opts ...Option) (T, error) {
	return DoJSON[T](ctx, Default, http.MethodPatch, rawURL, opts...)
}
func DeleteJSON[T any](ctx context.Context, rawURL string, opts ...Option) (T, error) {
	return DoJSON[T](ctx, Default, http.MethodDelete, rawURL, opts...)
}

// ---------------- Core ----------------

func (c *Client) Do(ctx context.Context, method, rawURL string, opts ...Option) ([]byte, error) {
	req, err := buildRequest(ctx, method, rawURL, opts...)
	if err != nil {
		return nil, err
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(b)
		if len(msg) > 2048 {
			msg = msg[:2048] + "...(truncated)"
		}
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       msg,
		}
	}
	return b, nil
}

func DoJSON[T any](ctx context.Context, c *Client, method, rawURL string, opts ...Option) (T, error) {
	var zero T

	if c == nil {
		c = Default
	}

	opts = append(opts, ExpectJSON())

	b, err := c.Do(ctx, method, rawURL, opts...)
	if err != nil {
		return zero, err
	}

	// 允许 T 是 []byte 或 string：直接返回
	switch any(zero).(type) {
	case []byte:
		return any(b).(T), nil
	case string:
		return any(string(b)).(T), nil
	}

	if len(bytes.TrimSpace(b)) == 0 {
		return zero, nil
	}

	if err := json.Unmarshal(b, &zero); err != nil {
		return zero, fmt.Errorf("json unmarshal failed: %w; body=%s", err, safeSnippet(b, 2048))
	}
	return zero, nil
}

// ---------------- Errors ----------------

type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http status %d (%s): %s", e.StatusCode, e.Status, e.Body)
}

// ---------------- Helpers ----------------

func buildRequest(ctx context.Context, method, rawURL string, opts ...Option) (*http.Request, error) {
	if ctx == nil {
		return nil, errors.New("ctx is nil")
	}
	if method == "" {
		return nil, errors.New("method is empty")
	}
	if rawURL == "" {
		return nil, errors.New("url is empty")
	}

	o := &requestOptions{
		header: make(http.Header),
		query:  make(url.Values),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	for k, vs := range o.query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()

	var body io.Reader
	switch {
	case o.rawBody != nil:
		body = o.rawBody
	case o.jsonBody != nil:
		b, err := json.Marshal(o.jsonBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
		if o.header.Get("Content-Type") == "" {
			o.header.Set("Content-Type", "application/json")
		}
	case o.form != nil:
		body = strings.NewReader(o.form.Encode())
		if o.header.Get("Content-Type") == "" {
			o.header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	for k, vs := range o.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	if o.expectJSON && req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	return req, nil
}

func safeSnippet(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
