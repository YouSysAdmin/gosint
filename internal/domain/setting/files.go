// Gosint, Copyright (c) 2026 YouSysAdmin

package setting

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/blob/db"
	"github.com/yousysadmin/gosint/internal/blob/fs"
	blobs3 "github.com/yousysadmin/gosint/internal/blob/s3"
	"github.com/yousysadmin/gosint/internal/blob/sealed"
	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
)

// pingTimeout bounds the check of a bucket before it is saved.
const pingTimeout = 15 * time.Second

// filesValue is the file store as stored, the S3 secret key included.
type filesValue struct {
	Backend     string  `json:"backend"`
	MaxSizeMB   int64   `json:"max_size_mb"`
	RetainHours int     `json:"retain_hours"`
	Dir         string  `json:"dir"`
	S3          s3Value `json:"s3"`
}

// s3Value is an S3-compatible bucket.
type s3Value struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	PathStyle bool   `json:"path_style"`
}

// defaultFiles is the store of an install that saved none: a directory
// beside the sqlite database, the database itself on postgres.
func defaultFiles(rt *env.Runtime) filesValue {
	d := env.DefaultFileLimits()
	v := filesValue{Backend: env.FilesFS, MaxSizeMB: d.MaxSize >> 20, RetainHours: int(d.RetainDeleted / time.Hour),
		S3: s3Value{Region: "us-east-1"}}
	if rt.Config.DefaultFilesDir() == "" {
		v.Backend = env.FilesDB
	}

	return v
}

func readFiles(ctx context.Context, rt *env.Runtime) (filesValue, error) {
	v := defaultFiles(rt)
	_, err := rt.Store.Setting.Get(ctx, Files, &v)

	return v, err
}

func (v filesValue) limits() env.FileLimits {
	return env.FileLimits{MaxSize: v.MaxSizeMB << 20, RetainDeleted: time.Duration(v.RetainHours) * time.Hour}
}

// dir is the fs backend's directory.
func (v filesValue) dir(rt *env.Runtime) string {
	if v.Dir != "" {
		return v.Dir
	}

	return rt.Config.DefaultFilesDir()
}

// location names where the bytes are, so a refresh finding the same one
// keeps the open backend. Credentials are part of it: new keys reopen.
func (v filesValue) location(rt *env.Runtime) string {
	switch v.Backend {
	case env.FilesS3:
		s := v.S3

		return strings.Join([]string{"s3", s.Endpoint, s.Region, s.Bucket, s.Prefix, s.AccessKey, s.SecretKey,
			fmt.Sprint(s.PathStyle)}, "\x00")
	case env.FilesDB:
		return "db"
	default:
		return "fs\x00" + v.dir(rt)
	}
}

// open builds the backend v names.
func (v filesValue) open(ctx context.Context, rt *env.Runtime) (sealed.Inner, error) {
	switch v.Backend {
	case env.FilesS3:
		s := v.S3

		return blobs3.New(ctx, blobs3.Options{Endpoint: s.Endpoint, Region: s.Region, Bucket: s.Bucket,
			Prefix: s.Prefix, AccessKey: s.AccessKey, SecretKey: s.SecretKey, PathStyle: s.PathStyle})
	case env.FilesDB:
		return db.New(rt.DB, ""), nil
	default:
		dir := v.dir(rt)
		if dir == "" {
			return nil, fmt.Errorf("files: the fs backend needs a directory on postgres")
		}

		return fs.New(dir)
	}
}

// fileSwitch is the server's sealed.Switch. A test's plain store
// cannot take a new backend.
type fileSwitch interface {
	Key() string
	Set(inner sealed.Inner, key string)
}

// applyFiles puts v to use on this node, opening the backend only when
// its location changed.
func applyFiles(ctx context.Context, rt *env.Runtime, v filesValue) error {
	rt.FileLimits.Set(v.limits())

	sw, ok := rt.Blobs.(fileSwitch)
	if !ok {
		return nil
	}

	key := v.location(rt)
	if sw.Key() == key {
		return nil
	}

	inner, err := v.open(ctx, rt)
	if err != nil {
		return err
	}

	sw.Set(inner, key)
	rt.Log.Info("files: the file store is in use", "backend", v.Backend)

	return nil
}

func filesView(rt *env.Runtime, v filesValue) filesResponse {
	return filesResponse{Files: FilesView{Backend: v.Backend, MaxSizeMB: v.MaxSizeMB, RetainHours: v.RetainHours,
		Dir: v.Dir, DefaultDir: rt.Config.DefaultFilesDir(),
		S3: S3View{Endpoint: v.S3.Endpoint, Region: v.S3.Region, Bucket: v.S3.Bucket, Prefix: v.S3.Prefix,
			AccessKey: v.S3.AccessKey, HasSecretKey: v.S3.SecretKey != "", PathStyle: v.S3.PathStyle}}}
}

// GetFiles answers the file store, the S3 secret as has_secret_key.
func (h Handler) GetFiles(c fiber.Ctx) error {
	v, err := readFiles(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, filesView(h.Runtime, v))
}

// PutFiles checks the store answers, saves it and puts it to use on
// this node. The files already stored stay where they were: moving them
// to a new backend is the administrator's.
func (h Handler) PutFiles(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[filesInput](c)
	if !ok {
		return resp
	}

	v, err := readFiles(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	v.Backend, v.MaxSizeMB, v.RetainHours, v.Dir = in.Backend, in.MaxSizeMB, in.RetainHours, in.Dir
	secret := v.S3.SecretKey
	v.S3 = s3Value{Endpoint: in.S3.Endpoint, Region: in.S3.Region, Bucket: in.S3.Bucket, Prefix: strings.Trim(in.S3.Prefix, "/"),
		AccessKey: in.S3.AccessKey, SecretKey: secret, PathStyle: in.S3.PathStyle}
	if in.S3.SecretKey != nil {
		v.S3.SecretKey = *in.S3.SecretKey
	}

	if v.S3.AccessKey == "" {
		v.S3.SecretKey = ""
	}

	switch v.Backend {
	case env.FilesFS:
		if v.Dir != "" && !filepath.IsAbs(v.Dir) {
			return refuse(c, "dir", "abs", "the directory must be an absolute path")
		}

		if v.dir(h.Runtime) == "" {
			return refuse(c, "dir", "required", "on postgres the directory backend needs a directory every node shares")
		}
	case env.FilesS3:
		if v.S3.Bucket == "" {
			return refuse(c, "s3.bucket", "required", "a bucket is required")
		}

		if v.S3.AccessKey != "" && v.S3.SecretKey == "" {
			return refuse(c, "s3.secret_key", "required", "an access key needs its secret key")
		}
	}

	ctx, cancel := context.WithTimeout(c, pingTimeout)
	defer cancel()

	inner, err := v.open(ctx, h.Runtime)
	if err == nil {
		err = inner.Ping(ctx)
	}

	if err != nil {
		return response.BadRequest(c, "the file store does not answer: "+err.Error())
	}

	if err := h.Runtime.Store.Setting.Put(c, Files, v, access.User(c).ID); err != nil {
		return response.Internal(c, err)
	}

	if err := applyFiles(c, h.Runtime, v); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, filesView(h.Runtime, v))
}
