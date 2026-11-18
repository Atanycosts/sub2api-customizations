package requesttiming

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A local synthetic comparison, not an upstream/model latency benchmark. Cold
// closes idle connections per sample; warm retains the transport's connection.
func BenchmarkRequestTimingHalfMiB(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	body := bytes.Repeat([]byte("x"), 512*1024)
	for _, cold := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			name := "warm/plain"
			if cold {
				name = "cold/plain"
			}
			if enabled {
				if cold {
					name = "cold/timing"
				} else {
					name = "warm/timing"
				}
			}
			b.Run(name, func(b *testing.B) {
				defaultTransport, ok := http.DefaultTransport.(*http.Transport)
				if !ok {
					b.Fatal("benchmark requires the standard HTTP transport")
				}
				tr := defaultTransport.Clone()
				defer tr.CloseIdleConnections()
				client := &http.Client{Transport: tr}
				if enabled {
					client = InstrumentClient(client, 19)
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				b.ResetTimer()
				for b.Loop() {
					if cold {
						tr.CloseIdleConnections()
					}
					ctx := context.Background()
					var c *Collector
					if enabled {
						ctx, c = New(ctx, "bench", "/v1/chat/completions")
					}
					req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader(body))
					resp, err := client.Do(req)
					if err != nil {
						b.Fatal(err)
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					if c != nil {
						c.Finish(200, false)
					}
				}
			})
		}
	}
}
