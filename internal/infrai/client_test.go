package infrai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureBucketAcceptsExistingBucket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/storage/bucket/create" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"STORAGE_BUCKET_EXISTS","message":"already exists"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureBucket(context.Background(), "care-private-files"); err != nil {
		t.Fatalf("EnsureBucket() error = %v", err)
	}
}
