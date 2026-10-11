// Gosint, Copyright (c) 2026 YouSysAdmin

// Package transformdir loads transforms from the files of a directory
// the operator edits: a YAML template (the template package's format)
// or a YAML manifest with a run line, which runs a script. The
// directory is read again when asked after a short while, so a file
// dropped in shows up without a restart, on every install alike.
package transformdir

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/yousysadmin/gosint/internal/core/template"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
)

// DefaultRescan is how long a scan of the directory stays current.
const DefaultRescan = 2 * time.Second

// Dir is one directory of transform files.
type Dir struct {
	// Path is the directory.
	Path string

	// Reserved are names taken by built-in transforms: a file naming one
	// is left out and reported, since the built-in wins.
	Reserved []string

	// Rescan is how long a scan stays current, DefaultRescan when zero.
	Rescan time.Duration

	types *types.Registry
	log   *slog.Logger

	mu     sync.Mutex
	last   time.Time
	files  map[string]entry
	loaded []transform.Transform
}

// entry is one file as last read.
type entry struct {
	mod  time.Time
	size int64
	t    transform.Transform
	err  error
}

// Loaded is a transform read from a file, which knows its file name.
type Loaded struct {
	transform.Transform

	file string
}

// File is the file the transform was read from, its base name.
func (l Loaded) File() string { return l.file }

// New makes a Dir over path. Nothing is read until Transforms is called.
func New(path string, reg *types.Registry, log *slog.Logger) *Dir {
	return &Dir{Path: path, types: reg, log: log, files: map[string]entry{}}
}

// Transforms is the directory's transforms as of now, the directory
// read again when the last read is older than Rescan. A file that does
// not parse is left out and logged once per change of the file.
func (d *Dir) Transforms() []transform.Transform {
	d.mu.Lock()
	defer d.mu.Unlock()

	rescan := d.Rescan
	if rescan == 0 {
		rescan = DefaultRescan
	}

	if time.Since(d.last) >= rescan {
		d.scan()
		d.last = time.Now()
	}

	return slices.Clone(d.loaded)
}

// scan reads the directory, parsing the files that changed since the
// last scan and dropping the ones gone.
func (d *Dir) scan() {
	entries, err := os.ReadDir(d.Path)
	if err != nil {
		if len(d.files) > 0 || d.loaded != nil {
			d.log.Warn("transforms directory", "dir", d.Path, "err", err)
		}

		d.files, d.loaded = map[string]entry{}, nil

		return
	}

	seen := map[string]bool{}
	changed := false
	for _, e := range entries {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") || strings.HasPrefix(name, ".") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		seen[name] = true
		if prev, ok := d.files[name]; ok && prev.mod.Equal(info.ModTime()) && prev.size == info.Size() {
			continue
		}

		t, err := d.parse(name)
		if err != nil {
			d.log.Warn("transform file left out", "file", name, "err", err)
		}

		d.files[name] = entry{mod: info.ModTime(), size: info.Size(), t: t, err: err}
		changed = true
	}

	for name := range d.files {
		if !seen[name] {
			delete(d.files, name)
			changed = true
		}
	}

	if changed || d.loaded == nil {
		d.rebuild()
	}
}

// rebuild lists the parsed files by name, the first of two files
// declaring the same transform name kept.
func (d *Dir) rebuild() {
	names := slices.Sorted(func(yield func(string) bool) {
		for name := range d.files {
			if !yield(name) {
				return
			}
		}
	})

	taken := map[string]string{}
	out := make([]transform.Transform, 0, len(names))
	for _, name := range names {
		e := d.files[name]
		if e.err != nil {
			continue
		}

		tname := e.t.Meta().Name
		if by, dup := taken[tname]; dup {
			d.log.Warn("transform file left out", "file", name, "err", "declares "+tname+" like "+by)

			continue
		}

		taken[tname] = name
		out = append(out, Loaded{Transform: e.t, file: name})
	}

	slices.SortFunc(out, func(a, b transform.Transform) int { return strings.Compare(a.Meta().Name, b.Meta().Name) })
	d.loaded = out
}

// parse reads one file: a manifest with a run line is a script, anything
// else a template.
func (d *Dir) parse(name string) (transform.Transform, error) {
	source, err := os.ReadFile(filepath.Join(d.Path, name)) //nolint:gosec // a file of the operator's directory
	if err != nil {
		return nil, err
	}

	var head struct {
		Name string `yaml:"name"`
		Run  string `yaml:"run"`
	}
	if err := yaml.NewDecoder(bytes.NewReader(source)).Decode(&head); err != nil {
		return nil, fmt.Errorf("not YAML: %w", err)
	}

	if slices.Contains(d.Reserved, strings.TrimSpace(head.Name)) {
		return nil, errors.New("declares " + head.Name + ", the name of a built-in transform")
	}

	if strings.TrimSpace(head.Run) != "" {
		m, err := ParseManifest(source, d.types)
		if err != nil {
			return nil, err
		}

		return Script{M: m, Dir: d.Path}, nil
	}

	tpl, err := template.Parse(source, d.types)
	if err != nil {
		return nil, err
	}

	return template.Transform{T: tpl}, nil
}
