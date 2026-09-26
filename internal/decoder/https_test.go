package decoder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPSPolicy(t *testing.T) {
	for _, raw := range []string{"http://host/audio", "https://user:pass@host/audio", "https://host/audio#secret"} {
		if _, err := (HTTPS{}).Open(context.Background(), raw, func() {}); err == nil {
			t.Fatal(raw)
		}
	}
	for _, status := range []int{401, 403, 404, 302, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "" || r.URL.Query().Get("signature") != "secret" {
					t.Error("authorization contract")
				}
				w.Header().Set("Location", "/leak")
				w.WriteHeader(status)
			}))
			defer s.Close()
			h := HTTPS{Client: s.Client(), Backoff: time.Millisecond}
			_, err := h.Open(context.Background(), s.URL+"/audio?signature=secret", func() {})
			want := int32(1)
			if status == 503 {
				want = 3
			}
			if err == nil || strings.Contains(err.Error(), "secret") || calls.Load() != want {
				t.Fatalf("calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestHTTPSLimitsAndTimeout(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if chunked {
				w.(http.Flusher).Flush()
			}
			w.Write([]byte(strings.Repeat("a", 1025)))
		}))
		_, err := (HTTPS{Client: s.Client(), MaxBytes: 1024}).Open(context.Background(), s.URL, func() {})
		s.Close()
		if err == nil || !strings.Contains(err.Error(), "limite") {
			t.Fatal(err)
		}
	}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	start := time.Now()
	_, err := (HTTPS{Client: s.Client(), Timeout: 20 * time.Millisecond, Backoff: time.Millisecond}).Open(context.Background(), s.URL, func() {})
	if err == nil || time.Since(start) > time.Second {
		t.Fatal("timeout", err)
	}
}
