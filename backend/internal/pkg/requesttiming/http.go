package requesttiming

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
)

type transport struct {
	base      http.RoundTripper
	accountID int64
}

// InstrumentClient preserves connection pools and attaches clocks to each actual
// RoundTrip, including redirects and transport-level compatibility retries.
func InstrumentClient(client *http.Client, accountID int64) *http.Client {
	copyClient := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copyClient.Transport = &transport{base, accountID}
	return &copyClient
}
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	c := FromContext(req.Context())
	if c == nil {
		return t.base.RoundTrip(req)
	}
	// Only a fixed endpoint class is retained, never URL query/path identifiers.
	endpoint := "other"
	switch req.URL.Path {
	case "/v1/chat/completions", "/chat/completions":
		endpoint = "chat_completions"
	case "/v1/responses", "/responses", "/v1/responses/compact":
		endpoint = "responses"
	}
	i := c.BeginAttempt(t.accountID, endpoint)
	trace := &httptrace.ClientTrace{
		GetConn:  func(string) { c.Event(i, "get_connection") },
		GotConn:  func(v httptrace.GotConnInfo) { c.Event(i, "got_connection"); c.Connection(i, v.Reused) },
		DNSStart: func(httptrace.DNSStartInfo) { c.Event(i, "dns_start") }, DNSDone: func(httptrace.DNSDoneInfo) { c.Event(i, "dns_end") },
		ConnectStart: func(string, string) { c.Event(i, "connect_start") }, ConnectDone: func(string, string, error) { c.Event(i, "connect_end") },
		TLSHandshakeStart: func() { c.Event(i, "tls_start") }, TLSHandshakeDone: func(tls.ConnectionState, error) { c.Event(i, "tls_end") },
		WroteHeaders: func() { c.Event(i, "wrote_headers") }, WroteRequest: func(v httptrace.WroteRequestInfo) {
			if v.Err == nil {
				c.Event(i, "upload_complete")
			} else {
				c.Event(i, "upload_failed")
			}
		},
		GotFirstResponseByte: func() { c.Event(i, "first_response_byte") },
	}
	resp, err := t.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		c.EndAttempt(i, "transport_error")
		return resp, err
	}
	c.Headers(i, resp.StatusCode)
	resp.Body = &observedBody{ReadCloser: resp.Body, c: c, index: i, status: resp.StatusCode}
	return resp, nil
}

type observedBody struct {
	io.ReadCloser
	c             *Collector
	index, status int
	once          sync.Once
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		outcome := "read_error"
		if err == io.EOF {
			outcome = "body_complete"
			if b.status >= 400 {
				outcome = "http_error"
			}
		}
		b.once.Do(func() { b.c.EndAttempt(b.index, outcome) })
	}
	return n, err
}
func (b *observedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.c.EndAttempt(b.index, "body_closed") })
	return err
}
