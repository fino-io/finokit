package storage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Object represents the metadata of an object.
type Object struct {
	LastModified time.Time `json:"lastModified,omitempty"`
	ETag         string    `json:"etag,omitempty"`
	Key          string    `json:"key,omitempty"`
	ContentType  string    `json:"contentType,omitempty"`
	Content      []byte    `json:"content,omitempty"`
	Size         int64     `json:"size,omitempty"`
}

// ListResult contains one page of listed objects.
type ListResult struct {
	Objects       []Object
	NextPageToken string
}

// var _ fs.FileInfo = (*Object)(nil)
// func (x *Object) Name() string       { return x.Key }
// func (x *Object) Size() int64        { return x.Size }
// func (x *Object) Mode() fs.FileMode  { return 0o644 }
// func (x *Object) ModTime() time.Time { return x.LastModified }
// func (x *Object) IsDir() bool        { return false }
// func (x *Object) Sys() any           { return "s3" }

// HTTPHeaders adapts an Object to HTTP headers.
func HTTPHeaders(x *Object) http.Header {
	headers := http.Header{}
	if x == nil {
		return headers
	}

	if len(x.ContentType) > 0 {
		headers.Set("Content-Type", x.ContentType)
		if strings.Contains(x.ContentType, "zip") {
			headers.Set("Content-Encoding", "gzip")
		}
	}

	if len(x.ETag) > 0 {
		headers.Set("ETag", x.ETag)
	}

	return headers
}

// WriteHTTPResponse writes an Object through the HTTP adapter.
func WriteHTTPResponse(writer http.ResponseWriter, x *Object) error {
	if x == nil {
		return nil
	}

	for key, values := range HTTPHeaders(x) {
		for _, value := range values {
			writer.Header().Set(key, value)
		}
	}

	if count, err := writer.Write(x.Content); err != nil {
		return err
	} else if count < len(x.Content) {
		return fmt.Errorf("failed to write completed content, expect :%d, actual: %d", len(x.Content), count)
	}

	return nil
}

func (x *Object) GetHttpHeaders() *http.Header {
	headers := HTTPHeaders(x)
	return &headers
}

func (x *Object) WriteHttpResponse(ctx context.Context, writer http.ResponseWriter) error {
	_ = ctx
	return WriteHTTPResponse(writer, x)
}
