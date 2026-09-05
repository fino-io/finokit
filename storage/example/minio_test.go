//go:build local
// +build local

package example

import (
	"context"
	"path"
	"reflect"
	"testing"

	"github.com/fino-io/finokit/storage"
)

var ctx = context.Background()

func newStorage(t *testing.T) storage.Storage {
	t.Helper()
	client, err := storage.New()
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestWrite(t *testing.T) {
	type args struct {
		object *storage.Object
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "write",
			args: args{
				object: &storage.Object{
					ETag:    "",
					Key:     path.Join("test", "write_test1"),
					Size:    int64(len("write test 1")),
					Content: []byte("write test 1"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := newStorage(t).Write(ctx, tt.args.object); (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpload(t *testing.T) {
	type args struct {
		localFile string
		key       string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "upload",
			args: args{
				localFile: "./config/storage.yaml",
				key:       path.Join("test", "storage.yaml"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := newStorage(t).Upload(ctx, tt.args.localFile, tt.args.key); (err != nil) != tt.wantErr {
				t.Errorf("Upload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPresignedUploadURL(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "upload",
			args: args{
				key: "presigned/write_test1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signURL, err := newStorage(t).PresignedUploadURL(ctx, tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("PresignedUploadURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			t.Logf("PresignedUploadURL() url = %s", signURL)
		})
	}
}

func TestPresignedDownloadURL(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "download",
			args: args{
				key: path.Join("test", "write_test1"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signURL, err := newStorage(t).PresignedDownloadURL(ctx, tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("PresignedDownloadURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			t.Logf("PresignedDownloadURL() url = %s", signURL)
		})
	}
}

func TestRead(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name        string
		args        args
		wantContent []byte
		wantErr     bool
	}{
		{
			name: "read",
			args: args{
				key: path.Join("test", "write_test1"),
			},
			wantContent: []byte("write test 1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newStorage(t).Read(ctx, tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Content, tt.wantContent) {
				t.Errorf("Read() got = %s, wantContent %s", got.Content, tt.wantContent)
			}
		})
	}
}

func TestDownload(t *testing.T) {
	type args struct {
		key  string
		path string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "download",
			args: args{
				key:  path.Join("test", "write_test1"),
				path: "./write_test2",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := newStorage(t).Download(ctx, tt.args.key, tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("Download() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
