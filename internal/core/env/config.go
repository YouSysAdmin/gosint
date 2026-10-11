// Gosint, Copyright (c) 2026 YouSysAdmin

// Package env owns Config (Viper-backed YAML loader) and the
// per-process Runtime that handlers receive.
package env

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/spf13/viper"
	tlsutils "github.com/yousysadmin/go-tlsutils"
)

// ConsolePath is where the web console is served, and the prefix every
// absolute link into it is built from.
const ConsolePath = "/app"

// Database drivers.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

// Config is what an operator can set: the YAML file merged with
// GOSINT_* environment overrides.
type Config struct {
	// Source describes where these values came from, for the boot log.
	Source string `mapstructure:"-"`

	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Auth       AuthConfig       `mapstructure:"auth"`
	Vault      VaultConfig      `mapstructure:"vault"`
	Transforms TransformsConfig `mapstructure:"transforms"`
	Console    ConsoleConfig    `mapstructure:"console"`
}

// The file backends.
const (
	// FilesFS keeps the bytes in a directory, beside the database by
	// default.
	FilesFS = "fs"

	// FilesS3 keeps them in an S3-compatible bucket.
	FilesS3 = "s3"

	// FilesDB keeps them in the database's blobs table, for a postgres
	// fleet with no bucket to share.
	FilesDB = "db"
)

// FileLimits bound the files attached to entities, set with the file
// store in the admin panel.
type FileLimits struct {
	// MaxSize bounds one upload, in bytes.
	MaxSize int64

	// RetainDeleted is how long the bytes of a deleted file are kept,
	// so an undo brings it back. The undo journal is pruned to it too.
	RetainDeleted time.Duration
}

// DefaultFileLimits are the limits of an install that saved none.
func DefaultFileLimits() FileLimits {
	return FileLimits{MaxSize: 32 << 20, RetainDeleted: 168 * time.Hour}
}

// ConsoleConfig is what the browser console is told to load.
type ConsoleConfig struct {
	// MapStyleLight and MapStyleDark are MapLibre style URLs for the map
	// view, fetched by the browser. Empty draws the points on a blank
	// background, for an install that must not reach a tile server.
	MapStyleLight string `mapstructure:"map_style_light"`
	MapStyleDark  string `mapstructure:"map_style_dark"`
}

// VaultConfig is how the master key everything is sealed under is
// given to the server. By default it is not: the server starts sealed
// and waits for the unseal key.
type VaultConfig struct {
	// UnsealKey unseals at boot (GOSINT_VAULT_UNSEAL_KEY), and
	// initializes a server that has no key yet.
	UnsealKey string `mapstructure:"unseal_key"`

	// UnsealKeyFile is UnsealKey read from a file.
	UnsealKeyFile string `mapstructure:"unseal_key_file"`

	// MasterKey is the raw key, 32 bytes of base64, "base64:" prefix
	// allowed, for development and tests: it unseals at boot.
	MasterKey string `mapstructure:"master_key"`
}

// TransformsConfig is where the operator's own transforms come from.
type TransformsConfig struct {
	// Dir holds transform files the server reads while running: YAML
	// templates and manifests running a script. Empty means a transforms
	// directory beside the sqlite database, and none on postgres.
	Dir string `mapstructure:"dir"`
}

// AuthConfig governs sign-in.
type AuthConfig struct {
	// SessionTTL is how long a sign-in lasts. Use renews it once half of
	// it has passed.
	SessionTTL time.Duration `mapstructure:"session_ttl"`

	// SessionMaxLife caps a sign-in however often it is renewed, so a
	// stolen cookie does not live for ever. Zero lifts the cap.
	SessionMaxLife time.Duration `mapstructure:"session_max_life"`

	// OpenRegistration lets anyone reaching the console create an
	// account. Off by default, which still admits the first account so
	// a fresh install can be claimed.
	OpenRegistration bool `mapstructure:"open_registration"`

	// PasskeysEnabled offers WebAuthn passkeys as a sign-in. On by
	// default, localhost is a secure context too.
	PasskeysEnabled bool `mapstructure:"passkeys_enabled"`

	// RateLimitPerMinute bounds the sign-in, registration and reset
	// requests one address may make in a minute.
	RateLimitPerMinute int `mapstructure:"rate_limit_per_minute"`
}

// ServerConfig is the HTTP listener.
type ServerConfig struct {
	Addr string `mapstructure:"addr"`

	// PublicURL is the address people reach the console at, when it is
	// not the host the request arrived on (behind a proxy rewriting it).
	PublicURL string `mapstructure:"public_url"`

	// TrustedProxies are the peers whose forwarding headers are read.
	TrustedProxies []string `mapstructure:"trusted_proxies"`

	// TLS is whether this listener does the handshake itself. Off by
	// default, a proxy in front usually holds the certificate.
	TLS TLSConfig `mapstructure:"tls"`
}

// The TLS modes, go-tlsutils's own spellings.
const (
	// TLSNone serves plain HTTP.
	TLSNone = tlsutils.ModeNone

	// TLSManual serves a PEM pair the operator provides.
	TLSManual = tlsutils.ModeManual

	// TLSSelf generates a self-signed pair and keeps it on disk.
	TLSSelf = tlsutils.ModeSelf
)

// TLSConfig is the server.tls block, mapped onto go-tlsutils by
// Config.TLSOptions.
type TLSConfig struct {
	// Mode is none, manual or self. Empty is none.
	Mode string `mapstructure:"mode"`

	// Cert and Key are the PEM pair, for manual.
	Cert string `mapstructure:"cert"`
	Key  string `mapstructure:"key"`

	// Watch reloads the pair when the files change, for manual, so a
	// renewal needs no restart.
	Watch bool `mapstructure:"watch"`

	// FQDN is the name on the generated certificate, for self. Empty
	// takes the host of public_url, else localhost.
	FQDN string `mapstructure:"fqdn"`

	// Alg is rsa, ecdsa or ed25519, for self. Empty is rsa.
	Alg string `mapstructure:"alg"`

	// CacheDir keeps the generated pair across restarts, for self.
	// Empty is a tls directory beside the sqlite database.
	CacheDir string `mapstructure:"cache_dir"`
}

// DatabaseConfig selects the backend.
//
// sqlite is the single-user local install, postgres is the team
// install. Both run the same queries.
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`

	// DSN is a file path for sqlite and a postgres:// URL for postgres.
	DSN string `mapstructure:"dsn"`

	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

// LoggingConfig is the single log sink.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
	Color  bool   `mapstructure:"color"`
}

// Load reads the YAML file at path (or ./gosint.yaml when path is
// empty), merges in GOSINT_* environment overrides, and returns the
// resolved Config. A missing file is not an error.
func Load(path string) (*Config, error) {
	v := viper.New()
	if path != "" {
		v.SetConfigFile(path)
	} else {
		// SetConfigFile and not SetConfigName: the name form also tries
		// the bare name, which would read the gosint binary as YAML.
		v.SetConfigFile("gosint.yaml")
	}

	v.SetEnvPrefix("GOSINT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// AutomaticEnv only resolves keys viper already knows about, so an
	// env-only value needs an explicit BindEnv per key.
	bindEnvKeys(v, reflect.TypeFor[Config](), "")

	v.SetDefault("server.addr", "127.0.0.1:4000")
	v.SetDefault("server.tls.mode", TLSNone)
	v.SetDefault("server.tls.watch", false)
	v.SetDefault("database.driver", DriverSQLite)
	v.SetDefault("database.dsn", "data/gosint.db")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 25)
	v.SetDefault("database.conn_max_lifetime", "30m")
	v.SetDefault("database.conn_max_idle_time", "5m")
	v.SetDefault("auth.session_ttl", "168h")
	v.SetDefault("auth.session_max_life", "720h")
	v.SetDefault("auth.open_registration", false)
	v.SetDefault("auth.passkeys_enabled", true)
	v.SetDefault("auth.rate_limit_per_minute", 10)
	v.SetDefault("transforms.dir", "")
	v.SetDefault("console.map_style_light", "https://tiles.openfreemap.org/styles/positron")
	v.SetDefault("console.map_style_dark", "https://tiles.openfreemap.org/styles/dark")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.color", true)

	source := "file " + v.ConfigFileUsed()
	if err := v.ReadInConfig(); err != nil {
		// The implicit ./gosint.yaml may be absent. One that is there
		// but unreadable is an error, as is any file named explicitly.
		if path != "" || !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read config %s: %w", v.ConfigFileUsed(), err)
		}

		source = "environment only"
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	c.Source = source
	trimStringSlices(reflect.ValueOf(&c))

	return &c, nil
}

// trimStringSlices strips whitespace from every []string entry, since
// viper splits a comma separated env value without trimming.
func trimStringSlices(v reflect.Value) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}

		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	for _, f := range v.Fields() {
		if f.Kind() == reflect.Struct || f.Kind() == reflect.Pointer {
			trimStringSlices(f)

			continue
		}

		if f.Kind() != reflect.Slice || f.Type().Elem().Kind() != reflect.String || !f.CanSet() {
			continue
		}

		for j := range f.Len() {
			e := f.Index(j)
			e.SetString(strings.TrimSpace(e.String()))
		}
	}
}

// bindEnvKeys calls v.BindEnv for every leaf key of the config struct.
func bindEnvKeys(v *viper.Viper, t reflect.Type, prefix string) {
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}

		name := f.Tag.Get("mapstructure")
		if name == "-" {
			continue
		}

		if name == "" {
			name = strings.ToLower(f.Name)
		}

		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct {
			bindEnvKeys(v, ft, key)

			continue
		}

		_ = v.BindEnv(key)
	}
}

// DefaultCacheDir is where transforms keep downloaded reference data
// when the worker settings name no directory: beside the sqlite
// database, else the user cache directory.
func (c *Config) DefaultCacheDir() string {
	if c.Database.Driver != DriverPostgres {
		return filepath.Join(filepath.Dir(c.Database.DSN), "cache")
	}

	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "gosint")
	}

	return filepath.Join(os.TempDir(), "gosint-cache")
}

// TransformsDir is where the operator's transform files live:
// transforms.dir, else a transforms directory beside the sqlite
// database, else "" for none.
func (c *Config) TransformsDir() string {
	if c.Transforms.Dir != "" {
		return c.Transforms.Dir
	}

	if c.Database.Driver != DriverPostgres {
		return filepath.Join(filepath.Dir(c.Database.DSN), "transforms")
	}

	return ""
}

// TLSOptions is the server.tls block as go-tlsutils takes it, with the
// name and the cache directory resolved.
func (c *Config) TLSOptions() tlsutils.Config {
	t := c.Server.TLS

	return tlsutils.Config{
		Mode:     t.Mode,
		Cert:     t.Cert,
		Key:      t.Key,
		Watch:    t.Watch,
		FQDN:     c.TLSHost(),
		Alg:      t.Alg,
		CacheDir: c.TLSCacheDir(),
	}
}

// TLSHost is the name a generated certificate is issued for:
// server.tls.fqdn, else the host of public_url, else localhost.
func (c *Config) TLSHost() string {
	if h := strings.TrimSpace(c.Server.TLS.FQDN); h != "" {
		return h
	}

	if u, err := url.Parse(c.Server.PublicURL); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}

	return "localhost"
}

// TLSCacheDir keeps the generated pair: server.tls.cache_dir, a tls
// directory beside the sqlite database, or the user config dir on
// postgres. Without one every start would serve a new certificate.
func (c *Config) TLSCacheDir() string {
	if c.Server.TLS.CacheDir != "" {
		return c.Server.TLS.CacheDir
	}

	if c.Database.Driver != DriverPostgres {
		return filepath.Join(filepath.Dir(c.Database.DSN), "tls")
	}

	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "gosint", "tls")
	}

	return filepath.Join(os.TempDir(), "gosint-tls")
}

// TerminatesTLS says this listener does the handshake itself.
func (c *Config) TerminatesTLS() bool {
	return c.TLSOptions().Enabled()
}

// validateTLS refuses a TLS block that would fail at bind time or half
// work.
func (c *Config) validateTLS() []error {
	opts := c.TLSOptions()
	if !slices.Contains([]string{"", TLSNone, TLSManual, TLSSelf}, opts.Mode) {
		return []error{fmt.Errorf("server.tls.mode must be %q, %q or %q, got %q", TLSNone, TLSManual, TLSSelf, opts.Mode)}
	}

	if err := opts.Validate(); err != nil {
		return []error{fmt.Errorf("server.tls: %w", err)}
	}

	// HSTS, the Secure cookies and every mailed link read public_url,
	// which must then say https.
	if opts.Enabled() && strings.HasPrefix(strings.ToLower(c.Server.PublicURL), "http://") {
		return []error{fmt.Errorf("server.tls.mode is %q but server.public_url is http, it has to be https", opts.Mode)}
	}

	return nil
}

// DefaultFilesDir is where the fs backend keeps files when the file
// store names no directory: beside the sqlite database, none on
// postgres, whose nodes need a directory they share or another backend.
func (c *Config) DefaultFilesDir() string {
	if c.Database.Driver == DriverPostgres {
		return ""
	}

	return filepath.Join(filepath.Dir(c.Database.DSN), "files")
}

// Validate refuses a config that would fail later.
func (c *Config) Validate() error {
	var errs []error
	if !slices.Contains([]string{DriverSQLite, DriverPostgres}, c.Database.Driver) {
		errs = append(errs, fmt.Errorf("database.driver must be %q or %q, got %q",
			DriverSQLite, DriverPostgres, c.Database.Driver))
	}

	if c.Database.DSN == "" {
		errs = append(errs, errors.New("database.dsn is required"))
	}

	// Mailed links and the cross-site check read it, a malformed one
	// would make the first relative and switch the second off.
	if c.Server.PublicURL != "" {
		u, err := url.Parse(c.Server.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("server.public_url must be an http or https URL with a host, got %q", c.Server.PublicURL))
		}
	}

	errs = append(errs, c.validateTLS()...)

	if c.Auth.SessionTTL < time.Minute {
		errs = append(errs, errors.New("auth.session_ttl must be at least 1m"))
	}

	if c.Auth.SessionMaxLife != 0 && c.Auth.SessionMaxLife < c.Auth.SessionTTL {
		errs = append(errs, errors.New("auth.session_max_life must be zero or at least auth.session_ttl"))
	}

	if c.Auth.RateLimitPerMinute < 1 {
		errs = append(errs, errors.New("auth.rate_limit_per_minute must be at least 1"))
	}

	if c.Vault.UnsealKey != "" && c.Vault.UnsealKeyFile != "" {
		errs = append(errs, errors.New("give vault.unseal_key or vault.unseal_key_file, not both"))
	}

	if c.Server.Addr == "" {
		errs = append(errs, errors.New("server.addr is required"))
	}

	for key, raw := range map[string]string{
		"console.map_style_light": c.Console.MapStyleLight, "console.map_style_dark": c.Console.MapStyleDark,
	} {
		if u, err := url.Parse(raw); raw != "" && (err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "") {
			errs = append(errs, fmt.Errorf("%s must be an http(s) URL or empty", key))
		}
	}

	return errors.Join(errs...)
}
