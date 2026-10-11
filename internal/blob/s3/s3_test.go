// Gosint, Copyright (c) 2026 YouSysAdmin

package s3

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/blob/blobtest"
	"github.com/yousysadmin/gosint/internal/core/ids"
)

// The contract, against a real S3-compatible service. Skipped LOUDLY
// without one - a skip is not a pass, and CI runs this job against a
// RustFS container so that the green tick means the backend was
// checked. Each subtest gets its own prefix in the bucket, so the
// suite's "empty store" assumption holds without emptying anything.
//
//	GOSINT_TEST_S3_ENDPOINT=http://127.0.0.1:9000 GOSINT_TEST_S3_BUCKET=iwiki-test \
//	GOSINT_TEST_S3_ACCESS_KEY=rustfsadmin GOSINT_TEST_S3_SECRET_KEY=rustfsadmin go test ./internal/blob/s3/
func TestContract(t *testing.T) {
	endpoint := os.Getenv("GOSINT_TEST_S3_ENDPOINT")
	bucket := os.Getenv("GOSINT_TEST_S3_BUCKET")
	if endpoint == "" || bucket == "" {
		t.Skip("GOSINT_TEST_S3_ENDPOINT and GOSINT_TEST_S3_BUCKET are not set: the s3 blob contract was NOT checked")
	}

	blobtest.Run(t, func(t *testing.T) blob.Store {
		t.Helper()

		s, err := New(t.Context(), Options{
			Endpoint:  endpoint,
			Bucket:    bucket,
			Prefix:    "blobtest/" + ids.New(),
			AccessKey: os.Getenv("GOSINT_TEST_S3_ACCESS_KEY"),
			SecretKey: os.Getenv("GOSINT_TEST_S3_SECRET_KEY"),
			PathStyle: true,
			SpoolDir:  t.TempDir(),
		})
		if err != nil {
			t.Fatal(err)
		}

		if err := s.Ping(t.Context()); err != nil {
			t.Fatalf("Ping: %v", err)
		}

		return s
	})
}

// Accept-Encoding goes out but is NOT in the signature.
//
// What it caught: every call to a real installation behind a reverse
// proxy answered 403 SignatureDoesNotMatch while the same
// credentials worked from the AWS CLI. The only difference on the
// wire was this header in SignedHeaders - a proxy rewrites it, so
// the request that arrived was not the request that was signed.
//
// Hermetic: a listener that records what the client sent, so the
// property is checked without a bucket anywhere.
func TestAcceptEncodingIsSentButNotSigned(t *testing.T) {
	seen := make(chan http.Header, 4)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<ListBucketResult><KeyCount>0</KeyCount></ListBucketResult>`))
	}))
	defer srv.Close()

	store, err := New(t.Context(), Options{
		Endpoint:  srv.URL,
		Bucket:    "wiki",
		AccessKey: "an-access-key",
		SecretKey: "a-secret-key",
		PathStyle: true,
		SpoolDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Ping(t.Context()); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	headers := <-seen

	// Still sent, or Go's transport adds its own and decompresses
	// transparently - which leaves Content-Length unknown on a store
	// that forwards Range requests.
	if got := headers.Get("Accept-Encoding"); got == "" {
		t.Error("Accept-Encoding was not sent at all")
	}

	auth := headers.Get("Authorization")
	if auth == "" {
		t.Fatal("the request was not signed")
	}

	signed := signedHeadersOf(t, auth)
	if strings.Contains(signed, "accept-encoding") {
		t.Errorf("SignedHeaders = %q, want no accept-encoding", signed)
	}

	// The control: the headers that MUST stay signed.
	for _, want := range []string{"host", "x-amz-content-sha256", "x-amz-date"} {
		if !strings.Contains(signed, want) {
			t.Errorf("SignedHeaders = %q, want %s in it", signed, want)
		}
	}
}

// Ping asks for a LIST, so a refusal carries the service's own code
// rather than the empty body of a HEAD.
func TestPingReportsWhatTheServiceSaid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Ping used %s, which has no error body to report", r.Method)
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Error><Code>SignatureDoesNotMatch</Code><Message>nope</Message></Error>`))
	}))
	defer srv.Close()

	store, err := New(t.Context(), Options{
		Endpoint: srv.URL, Bucket: "wiki",
		AccessKey: "a", SecretKey: "b", PathStyle: true, SpoolDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = store.Ping(t.Context())
	if err == nil {
		t.Fatal("Ping said the bucket was fine")
	}

	if !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Errorf("Ping = %v, want the service's own code in it", err)
	}
}

// signedHeadersOf pulls the list out of an Authorization header.
func signedHeadersOf(t *testing.T, auth string) string {
	t.Helper()

	_, rest, ok := strings.Cut(auth, "SignedHeaders=")
	if !ok {
		t.Fatalf("no SignedHeaders in %q", auth)
	}

	list, _, _ := strings.Cut(rest, ",")

	return list
}
