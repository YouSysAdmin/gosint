// Gosint, Copyright (c) 2026 YouSysAdmin

package setting

import "github.com/yousysadmin/gosint/internal/core/tool"

// mailInput saves the relay. The password is never read back, so an
// absent one keeps the stored one.
type mailInput struct {
	Host     string  `json:"host"     validate:"max=255" normalize:"trim"`
	Port     int     `json:"port"`
	Username string  `json:"username" validate:"max=255" normalize:"trim"`
	Password *string `json:"password" validate:"omitnil,max=1000"`
	From     string  `json:"from"     validate:"max=320" normalize:"trim"`
	StartTLS bool    `json:"starttls"`
}

// MailView is the relay as the panel sees it: has_password, never the
// password.
type MailView struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	HasPassword bool   `json:"has_password"`
	From        string `json:"from"`
	StartTLS    bool   `json:"starttls"`
	Enabled     bool   `json:"enabled"`
}

type mailResponse struct {
	Mail MailView `json:"mail"`
}

type testResponse struct {
	SentTo string `json:"sent_to"`
}

type workerResponse struct {
	Worker workerValue `json:"worker"`

	// DefaultCacheDir is where downloads go when no directory is set.
	DefaultCacheDir string `json:"default_cache_dir"`
}

// toolsInput saves the tools it names.
type toolsInput struct {
	Tools map[string]tool.Entry `json:"tools" validate:"required,max=64"`
}

// ToolView is one tool as the panel sees it. Found is where the node
// answering finds the program, empty when it does not.
type ToolView struct {
	Name         string `json:"name"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Binary       string `json:"binary"`
	Path         string `json:"path"`
	AllowPrivate bool   `json:"allow_private"`
	Found        string `json:"found"`
}

type toolsResponse struct {
	Tools []ToolView `json:"tools"`
}

// filesInput saves the file store. The S3 secret key is never read
// back, absent keeps the stored one.
type filesInput struct {
	Backend     string  `json:"backend"      validate:"oneof=fs s3 db"`
	MaxSizeMB   int64   `json:"max_size_mb"  validate:"min=1,max=4096"`
	RetainHours int     `json:"retain_hours" validate:"min=1,max=87600"`
	Dir         string  `json:"dir"          validate:"max=1024" normalize:"trim"`
	S3          s3Input `json:"s3"`
}

type s3Input struct {
	Endpoint  string  `json:"endpoint"   validate:"max=400" normalize:"trim"`
	Region    string  `json:"region"     validate:"max=60" normalize:"trim"`
	Bucket    string  `json:"bucket"     validate:"max=255" normalize:"trim"`
	Prefix    string  `json:"prefix"     validate:"max=255" normalize:"trim"`
	AccessKey string  `json:"access_key" validate:"max=255" normalize:"trim"`
	SecretKey *string `json:"secret_key" validate:"omitnil,max=1000"`
	PathStyle bool    `json:"path_style"`
}

// FilesView is the file store as the panel sees it. DefaultDir is the
// directory used when none is set, empty on postgres.
type FilesView struct {
	Backend     string `json:"backend"`
	MaxSizeMB   int64  `json:"max_size_mb"`
	RetainHours int    `json:"retain_hours"`
	Dir         string `json:"dir"`
	DefaultDir  string `json:"default_dir"`
	S3          S3View `json:"s3"`
}

// S3View is the bucket, has_secret_key and never the key.
type S3View struct {
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	Prefix       string `json:"prefix"`
	AccessKey    string `json:"access_key"`
	HasSecretKey bool   `json:"has_secret_key"`
	PathStyle    bool   `json:"path_style"`
}

type filesResponse struct {
	Files FilesView `json:"files"`
}
