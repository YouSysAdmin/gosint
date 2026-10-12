// Gosint, Copyright (c) 2026 YouSysAdmin

// Package tool is what transforms use on the server beyond Go itself:
// a program such as nmap, or the client reaching what an entity names.
// An administrator sets each one's path and whether it may reach
// private addresses. A new tool is a type implementing Tool, added to
// the registry built at boot.
package tool

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
)

// Tool is one tool an administrator can set.
type Tool interface {
	// Name keys the tool's settings, fixed once released.
	Name() string
	Title() string

	// Description says what uses the tool.
	Description() string

	// Binary is the program looked up on PATH when no path is set,
	// empty for a tool that is no program.
	Binary() string
}

// Entry is one tool's settings.
type Entry struct {
	// Path is the program, the binary on PATH when empty.
	Path string `json:"path"`

	// AllowPrivate lets the tool reach loopback, private and link-local
	// addresses, for a lab (TryHackMe, HackTheBox, a home network). Off,
	// a sketch cannot read the network the server sits in.
	AllowPrivate bool `json:"allow_private"`

	// Sandbox confines a transform file's script, nil is this node's
	// default: on where it can be.
	Sandbox *bool `json:"sandbox,omitempty"`

	// ReadPaths and WritePaths are what a confined script may read or
	// write beyond the system and its own directory, absolute.
	ReadPaths  []string `json:"read_paths,omitempty"`
	WritePaths []string `json:"write_paths,omitempty"`
}

// Confined tells whether scripts run in the sandbox: as set, else on
// exactly where the node supports it.
func (e Entry) Confined(supported bool) bool {
	if e.Sandbox != nil {
		return *e.Sandbox
	}

	return supported
}

// Settings are every tool's entries by name.
type Settings map[string]Entry

// Get is a tool's entry, the zero one when never set.
func (s Settings) Get(name string) Entry { return s[name] }

// Command is the program to run for t: its set path, else its binary.
func (s Settings) Command(t Tool) string {
	if p := strings.TrimSpace(s.Get(t.Name()).Path); p != "" {
		return p
	}

	return t.Binary()
}

// ErrNotFound is a program neither at its path nor on PATH.
var ErrNotFound = errors.New("not found")

// Locate is where the program for t is on this node.
func (s Settings) Locate(t Tool) (string, error) {
	cmd := s.Command(t)
	if cmd == "" {
		return "", ErrNotFound
	}

	found, err := exec.LookPath(cmd)
	if err != nil {
		return "", ErrNotFound
	}

	return found, nil
}

// Registry is the tools this build knows.
type Registry struct {
	tools []Tool
}

// NewRegistry builds a registry, in the order given.
func NewRegistry(tools ...Tool) *Registry {
	return &Registry{tools: slices.Clone(tools)}
}

// All is every tool.
func (r *Registry) All() []Tool { return slices.Clone(r.tools) }

// Get is one tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	i := slices.IndexFunc(r.tools, func(t Tool) bool { return t.Name() == name })
	if i < 0 {
		return nil, false
	}

	return r.tools[i], true
}

// WebName names the web client's settings.
const WebName = "web"

// Web is the client every transform reaching a site, a host or a whois
// server an entity names goes through, and the assistant's page
// fetches. No program, only the private address switch.
type Web struct{}

// Name implements Tool.
func (Web) Name() string { return WebName }

// Title implements Tool.
func (Web) Title() string { return "Web and network requests" }

// Description implements Tool.
func (Web) Description() string {
	return "Transforms fetching a site, a host or a whois server an entity names, and the assistant's page lookups"
}

// Binary implements Tool.
func (Web) Binary() string { return "" }

// ScriptsName names the settings of transform files' scripts.
const ScriptsName = "scripts"

// Scripts is the programs transform files run. No program of its own
// and no network switch: the sandbox and the paths it lets through.
type Scripts struct{}

// Name implements Tool.
func (Scripts) Name() string { return ScriptsName }

// Title implements Tool.
func (Scripts) Title() string { return "Transform file scripts" }

// Description implements Tool.
func (Scripts) Description() string {
	return "The scripts that transform files with a run line start, confined to the paths they need"
}

// Binary implements Tool.
func (Scripts) Binary() string { return "" }
