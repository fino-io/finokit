package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestObject_GetHttpHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		object    *Object
		wantLen   int
		wantType  string
		wantETag  string
		wantCodec string
	}{
		{
			name:    "nil object",
			object:  nil,
			wantLen: 0,
		},
		{
			name: "plain content type",
			object: &Object{
				ETag:         "1234",
				LastModified: time.Time{},
				ContentType:  "application/json",
			},
			wantLen:  2,
			wantType: "application/json",
			wantETag: "1234",
		},
		{
			name: "zip content type adds encoding",
			object: &Object{
				ETag:         "etag-1",
				LastModified: time.Time{},
				ContentType:  "application/zip",
			},
			wantLen:   3,
			wantType:  "application/zip",
			wantETag:  "etag-1",
			wantCodec: "gzip",
		},
		{
			name: "empty fields produce no headers",
			object: &Object{
				LastModified: time.Time{},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := tt.object.GetHttpHeaders()
			if headers == nil {
				t.Fatal("GetHttpHeaders() returned nil")
			}

			if got := len(*headers); got != tt.wantLen {
				t.Fatalf("GetHttpHeaders() len = %d, want %d", got, tt.wantLen)
			}

			if got := headers.Get("Content-Type"); got != tt.wantType {
				t.Fatalf("GetHttpHeaders() Content-Type = %q, want %q", got, tt.wantType)
			}

			if got := headers.Get("ETag"); got != tt.wantETag {
				t.Fatalf("GetHttpHeaders() ETag = %q, want %q", got, tt.wantETag)
			}

			if got := headers.Get("Content-Encoding"); got != tt.wantCodec {
				t.Fatalf("GetHttpHeaders() Content-Encoding = %q, want %q", got, tt.wantCodec)
			}
		})
	}
}

func TestObject_WriteHttpResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		object      *Object
		writer      http.ResponseWriter
		wantBody    string
		wantErr     string
		wantHeaders map[string]string
	}{
		{
			name:   "nil object is noop",
			object: nil,
			writer: httptest.NewRecorder(),
		},
		{
			name: "writes body and headers",
			object: &Object{
				ETag:        "etag-2",
				ContentType: "application/json",
				Content:     []byte(`{"ok":true}`),
			},
			writer:   httptest.NewRecorder(),
			wantBody: `{"ok":true}`,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
				"ETag":         "etag-2",
			},
		},
		{
			name: "short write returns error",
			object: &Object{
				Content: []byte("hello"),
			},
			writer:  &stubResponseWriter{header: make(http.Header), writeN: 2},
			wantErr: "failed to write completed content",
		},
		{
			name: "writer error bubbles up",
			object: &Object{
				Content: []byte("hello"),
			},
			writer:  &stubResponseWriter{header: make(http.Header), err: errors.New("write failed")},
			wantErr: "write failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.object.WriteHttpResponse(context.Background(), tt.writer)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("WriteHttpResponse() error = %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("WriteHttpResponse() error = nil, want non-nil")
				}
				if got := err.Error(); got == "" || !strings.Contains(got, tt.wantErr) {
					t.Fatalf("WriteHttpResponse() error = %q, want contains %q", got, tt.wantErr)
				}
			}

			recorder, ok := tt.writer.(*httptest.ResponseRecorder)
			if !ok {
				return
			}

			if got := recorder.Body.String(); got != tt.wantBody {
				t.Fatalf("WriteHttpResponse() body = %q, want %q", got, tt.wantBody)
			}

			for key, want := range tt.wantHeaders {
				if got := recorder.Header().Get(key); got != want {
					t.Fatalf("WriteHttpResponse() header %s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

type stubResponseWriter struct {
	header http.Header
	err    error
	writeN int
}

func (w *stubResponseWriter) Header() http.Header {
	return w.header
}

func (w *stubResponseWriter) Write(b []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.writeN > 0 {
		return w.writeN, nil
	}
	return len(b), nil
}

func (w *stubResponseWriter) WriteHeader(int) {}
