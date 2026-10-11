// Gosint, Copyright (c) 2026 YouSysAdmin

// Package s3 keeps blobs in an S3-compatible bucket, one object per
// hash at assets/ab/cd/<hash>, under the prefix when there is one.
// For the installation whose
// attachments outgrow a disk, or whose nodes do not share one.
//
// Everything is still served THROUGH the server: no presigned URLs, no
// public bucket. The access gates on a sketch apply to its files, and
// a Range from the browser is forwarded as a Range to the bucket, so
// seeking in a video costs one object read of the part asked for.
package s3

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/yousysadmin/gosint/internal/blob"
)

// Options is what New needs. It mirrors the attachments.s3 section
// without importing the config package, so this package is usable
// from a test with its own values.
type Options struct {
	// Endpoint is the service URL, empty for AWS.
	Endpoint string

	// Region is what the signature names.
	Region string

	// Bucket is the bucket. Required.
	Bucket string

	// Prefix is put in front of every key, no slashes at either end.
	Prefix string

	// AccessKey and SecretKey are static credentials. Both empty
	// means the default chain.
	AccessKey string
	SecretKey string

	// PathStyle puts the bucket in the path rather than the host.
	PathStyle bool

	// SpoolDir is where an upload is written while it is hashed.
	// Empty means the system temp dir.
	SpoolDir string
}

// Store is the bucket implementation.
type Store struct {
	client *awss3.Client
	bucket string
	prefix string
	spool  string
}

// New builds a client. Nothing is contacted here: Ping is the check,
// and boot calls it.
func New(ctx context.Context, o Options) (*Store, error) {
	if o.Bucket == "" {
		return nil, errors.New("s3: a bucket is required")
	}

	region := cmp.Or(o.Region, "us-east-1")
	httpClient := boundedHTTPClient()

	opts := func(c *awss3.Options) {
		if o.Endpoint != "" {
			c.BaseEndpoint = aws.String(o.Endpoint)
		}

		c.Region = region
		c.UsePathStyle = o.PathStyle
		c.HTTPClient = httpClient
		c.APIOptions = append(c.APIOptions, unsignAcceptEncoding)
	}

	var client *awss3.Client
	if o.AccessKey != "" {
		client = awss3.New(awss3.Options{
			Credentials: credentials.NewStaticCredentialsProvider(o.AccessKey, o.SecretKey, ""),
		}, opts)
	} else {
		// The DEFAULT CHAIN: environment, shared config, an instance
		// role over IMDS, a task role, a service account. A client
		// built with nil credentials resolves nothing and fails at
		// the first request, so this is the only other way to build
		// one.
		cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region), awsconfig.WithHTTPClient(httpClient))
		if err != nil {
			return nil, fmt.Errorf("s3: no credentials (set access_key and secret_key, or give this machine a role): %w", err)
		}

		client = awss3.NewFromConfig(cfg, opts)
	}

	spool := o.SpoolDir
	if spool == "" {
		spool = os.TempDir()
	}

	return &Store{client: client, bucket: o.Bucket, prefix: strings.Trim(o.Prefix, "/"), spool: spool}, nil
}

// The bounds on one conversation with the bucket. Every one of them
// is a wait for the OTHER side - none caps a whole transfer, so a
// large video streamed to a slow reader or a long upload is never cut
// off while bytes are still moving.
const (
	dialTimeout           = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = time.Minute

	// readIdleTimeout fails a connection that delivers nothing for
	// this long. It resets on every read, so a slow but progressing
	// body survives it, and a body nobody is reading is not timed.
	readIdleTimeout = time.Minute
)

// boundedHTTPClient is the SDK's own client with every wait bounded.
// The SDK's default leaves the response wait open, so a bucket that
// accepted a connection and then said nothing held the handler -
// and the request goroutine behind it - until the reader gave up.
func boundedHTTPClient() *awshttp.BuildableClient {
	return awshttp.NewBuildableClient().
		WithDialerOptions(func(d *net.Dialer) { d.Timeout = dialTimeout }).
		WithTransportOptions(func(tr *http.Transport) {
			tr.TLSHandshakeTimeout = tlsHandshakeTimeout
			tr.ResponseHeaderTimeout = responseHeaderTimeout
		}).
		WithReadTimeout(readIdleTimeout)
}

// key is the object name of one blob.
func (s *Store) key(hash string) (string, error) {
	if !blob.ValidHash(hash) {
		return "", blob.ErrBadHash
	}

	if s.prefix == "" {
		return blob.Shard(hash), nil
	}

	return s.prefix + "/" + blob.Shard(hash), nil
}

// Put spools and hashes the upload, then writes the object unless it
// is already there. The spool is unavoidable: the key is the hash, and
// the hash is not known until the end. HeadObject first, because on a
// duplicate the PUT of a large file is the expensive half.
func (s *Store) Put(ctx context.Context, r io.Reader, limit int64) (string, int64, error) {
	return s.put(ctx, "", r, limit)
}

// PutNamed is Put under a name the caller chose.
func (s *Store) PutNamed(ctx context.Context, name string, r io.Reader, limit int64) (int64, error) {
	if !blob.ValidHash(name) {
		return 0, blob.ErrBadHash
	}

	_, size, err := s.put(ctx, name, r, limit)

	return size, err
}

// put stores r under name, its own hash when name is empty.
func (s *Store) put(ctx context.Context, name string, r io.Reader, limit int64) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	f, hash, size, err := blob.Spool(s.spool, r, limit)
	if err != nil {
		return "", 0, err
	}

	if name != "" {
		hash = name
	}

	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()

	key, err := s.key(hash)
	if err != nil {
		return "", 0, err
	}

	exists, err := s.exists(ctx, key)
	if err != nil {
		return "", 0, err
	}

	// The object keeps its old LastModified. Refreshing it would mean
	// copying the object onto itself, a full server-side copy per
	// duplicate upload. What the time protects - an orphan the gc may
	// take just before this upload's record names it - the asset
	// domain covers instead, by checking the bytes are still there
	// once the record has landed.
	if exists {
		return hash, size, nil
	}

	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentLength: aws.Int64(size),
		// NO REAL TYPE ON THE OBJECT, and it is not an omission.
		// The bytes are addressed by their hash and shared by every
		// record that names them, while the type is decided from the
		// NAME - so one object genuinely has two right answers when
		// the same file is uploaded as a .gif and as a .txt, and
		// whichever upload came first would win. The type lives on
		// the record, which is what Serve sends. It is also the safe
		// answer if a bucket is ever made public by mistake: a
		// browser downloads octet-stream instead of running what a
		// sniffed text/html would have told it to run.
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return "", 0, fmt.Errorf("s3 put %s: %w", key, err)
	}

	return hash, size, nil
}

// Open reads the object, forwarding the range when there is one.
func (s *Store) Open(ctx context.Context, hash string, rng *blob.Range) (io.ReadCloser, blob.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, blob.Object{}, err
	}

	key, err := s.key(hash)
	if err != nil {
		return nil, blob.Object{}, blob.ErrNotFound
	}

	in := &awss3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if rng != nil {
		spec := "bytes=" + strconv.FormatInt(rng.Start, 10) + "-"
		if rng.End >= 0 {
			spec += strconv.FormatInt(rng.End, 10)
		}

		in.Range = aws.String(spec)
	}

	out, err := s.client.GetObject(ctx, in)
	if err != nil {
		if isNotFound(err) {
			return nil, blob.Object{}, blob.ErrNotFound
		}

		// The size is not in the error, so the Object carries none
		// and the caller answers 416 from what it knows itself.
		if unsatisfiable(err) {
			return nil, blob.Object{}, blob.ErrRange
		}

		return nil, blob.Object{}, fmt.Errorf("s3 get %s: %w", key, err)
	}

	obj, err := describe(out, rng)
	if err != nil {
		_ = out.Body.Close()

		return nil, blob.Object{}, err
	}

	return out.Body, obj, nil
}

// describe turns the response headers into an Object. A ranged
// response carries Content-Range with the whole size, a full one
// carries only Content-Length.
func describe(out *awss3.GetObjectOutput, rng *blob.Range) (blob.Object, error) {
	length := aws.ToInt64(out.ContentLength)
	if rng == nil || out.ContentRange == nil {
		return blob.Object{Size: length, Offset: 0, Length: length}, nil
	}

	// "bytes 4-7/16"
	spec, ok := strings.CutPrefix(aws.ToString(out.ContentRange), "bytes ")
	if !ok {
		return blob.Object{}, fmt.Errorf("s3: unexpected Content-Range %q", aws.ToString(out.ContentRange))
	}

	rangePart, sizePart, ok := strings.Cut(spec, "/")
	if !ok {
		return blob.Object{}, fmt.Errorf("s3: unexpected Content-Range %q", aws.ToString(out.ContentRange))
	}

	startPart, _, _ := strings.Cut(rangePart, "-")
	start, err := strconv.ParseInt(startPart, 10, 64)
	if err != nil {
		return blob.Object{}, fmt.Errorf("s3: unexpected Content-Range %q", aws.ToString(out.ContentRange))
	}

	size, err := strconv.ParseInt(sizePart, 10, 64)
	if err != nil {
		return blob.Object{}, fmt.Errorf("s3: unexpected Content-Range %q", aws.ToString(out.ContentRange))
	}

	return blob.Object{Size: size, Offset: start, Length: length}, nil
}

// Exists reports whether the object is there.
func (s *Store) Exists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// A malformed hash names no object, so it reads as missing rather
	// than as a failure of the store.
	key, err := s.key(hash)
	if err != nil {
		return false, nil
	}

	return s.exists(ctx, key)
}

func (s *Store) exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("s3 head %s: %w", key, err)
	}

	return true, nil
}

// Delete removes the object. S3 answers a delete of a missing key
// with success, which is the contract anyway.
func (s *Store) Delete(ctx context.Context, hash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// A malformed hash names no object, and deleting what is not
	// there is not an error.
	key, err := s.key(hash)
	if err != nil {
		return nil
	}

	_, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}

	return nil
}

// List pages through the prefix. An object whose last segment is not
// a hash is somebody else's and skipped.
func (s *Store) List(ctx context.Context, fn func(hash string, modified time.Time) error) error {
	prefix := blob.Dir + "/"
	if s.prefix != "" {
		prefix = s.prefix + "/" + prefix
	}

	p := awss3.NewListObjectsV2Paginator(s.client, &awss3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("s3 list: %w", err)
		}

		for _, o := range page.Contents {
			key := aws.ToString(o.Key)
			_, hash, _ := strings.CutLast(key, "/")
			if !blob.ValidHash(hash) {
				continue
			}

			// A bucket always reports this, but a nil is a nil:
			// the zero time reads as "long ago", which is the
			// answer that makes the gc treat it as an orphan
			// rather than skip it forever.
			var modified time.Time
			if o.LastModified != nil {
				modified = o.LastModified.UTC()
			}

			if err := fn(hash, modified); err != nil {
				return err
			}
		}
	}

	return nil
}

// Ping asks for the bucket. Wrong credentials, a missing bucket and
// an unreachable endpoint all fail here, at boot.
func (s *Store) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// A LIST and not a HeadBucket, for the error and not for the
	// answer. HEAD has no body, so every refusal arrives as the bare
	// word "Forbidden" - which is what an operator was handed for a
	// signature this client had got wrong, with nothing in it to act
	// on. The same call as ListObjectsV2 returns the service's own
	// code, and it needs no permission the gc did not already need.
	_, err := s.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return fmt.Errorf("s3 list %s: %w", s.bucket, err)
	}

	return nil
}

// isNotFound is the shape a missing object comes back in: a typed
// NoSuchKey or NotFound from GetObject, a bare 404 from HeadObject.
func isNotFound(err error) bool {
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return true
	}

	if _, ok := errors.AsType[*types.NotFound](err); ok {
		return true
	}

	if api, ok := errors.AsType[smithy.APIError](err); ok {
		switch api.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		}
	}

	return false
}

// unsatisfiable recognises a 416.
func unsatisfiable(err error) bool {
	api, ok := errors.AsType[smithy.APIError](err)

	return ok && api.ErrorCode() == "InvalidRange"
}

// acceptEncoding is signed by this SDK and NOT by the AWS CLI, which
// is the whole of the difference below.
const acceptEncoding = "Accept-Encoding"

// unsignAcceptEncoding keeps Accept-Encoding out of the signature
// while still sending it, which is what botocore does.
//
// aws-sdk-go-v2 sets `Accept-Encoding: identity` and lists it in
// SignedHeaders. That is legal - the server is told exactly which
// headers were signed - but it makes the signature depend on a
// header that HTTP infrastructure rewrites as a matter of routine: a
// reverse proxy with compression on will set or clear it upstream,
// and the request that arrives is then not the request that was
// signed. Measured against a real installation behind a proxy: every
// call answered 403 SignatureDoesNotMatch, the same credentials
// worked from the AWS CLI, and the ONLY difference on the wire was
// this header appearing in SignedHeaders.
//
// Removed before the signer runs and put back after, so what goes
// out is unchanged and only the signature is narrower. It is not
// removed outright: without the header Go's transport adds its own
// `Accept-Encoding: gzip` and transparently decompresses, which
// leaves Content-Length unknown - and this store forwards Range
// requests and reports sizes.
//
// Unconditional, because it is what the reference client does and
// AWS itself honours SignedHeaders. A setting here would be a knob
// nobody would know to reach for, offered at the moment they are
// looking at a bare 403.
func unsignAcceptEncoding(stack *middleware.Stack) error {
	var held string

	err := stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc("gosintUnsignAcceptEncoding",
		func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
			middleware.FinalizeOutput, middleware.Metadata, error,
		) {
			if req, ok := in.Request.(*smithyhttp.Request); ok {
				held = req.Header.Get(acceptEncoding)
				req.Header.Del(acceptEncoding)
			}

			return next.HandleFinalize(ctx, in)
		}), "Signing", middleware.Before)
	if err != nil {
		return err
	}

	return stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc("gosintRestoreAcceptEncoding",
		func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
			middleware.FinalizeOutput, middleware.Metadata, error,
		) {
			if req, ok := in.Request.(*smithyhttp.Request); ok && held != "" {
				req.Header.Set(acceptEncoding, held)
			}

			return next.HandleFinalize(ctx, in)
		}), "Signing", middleware.After)
}
