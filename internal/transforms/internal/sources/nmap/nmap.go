// Gosint, Copyright (c) 2026 YouSysAdmin

// Package nmap runs the nmap binary installed on the server and reads
// its report. It is the active port source: unlike InternetDB it
// connects to the target, so it runs only when asked for by name.
package nmap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/nmapreport"
	"github.com/yousysadmin/gosint/internal/core/transform"
)

const (
	// DefaultTopPorts is how many of the most common ports a scan
	// tries when not told otherwise.
	DefaultTopPorts = 100

	// MaxTopPorts bounds a scan, nmap's own list has about 8000.
	MaxTopPorts = 1000

	// Timeout bounds one host, well under the worker's lease.
	Timeout = 5 * time.Minute
)

// ToolName names nmap's settings.
const ToolName = "nmap"

// Tool is nmap as an administrator sets it: its path and whether it may
// scan private addresses.
type Tool struct{}

// Name implements tool.Tool.
func (Tool) Name() string { return ToolName }

// Title implements tool.Tool.
func (Tool) Title() string { return "Nmap" }

// Description implements tool.Tool.
func (Tool) Description() string {
	return "The active source of ip_to_ports, scanning the address itself"
}

// Binary implements tool.Tool.
func (Tool) Binary() string { return "nmap" }

// ErrNotInstalled is a scan on a server without the binary.
var ErrNotInstalled = errors.New("nmap is not installed on the server, install it, set its path under Administration, Tools, or use the passive source")

// Scanner runs nmap. Path is the binary, "nmap" found on PATH when
// empty.
type Scanner struct {
	Path string

	// AllowPrivate lifts the refusal of internal addresses, a lab setting.
	AllowPrivate bool
}

// Port is one open port of the report.
type Port struct {
	Number   int
	Protocol string
	State    string
	Service  string
	Banner   string
}

// Scan lists the open ports among the topPorts most common ones of
// ip. An internal address is refused, as the web client refuses it: a
// sketch must not map the network the server sits in.
func (s Scanner) Scan(ctx context.Context, ip string, topPorts int) ([]Port, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return nil, err
	}

	if !s.AllowPrivate && !transform.Public(addr) {
		return nil, fmt.Errorf("%w: %s", transform.ErrPrivateTarget, ip)
	}

	path := s.Path
	if path == "" {
		path = "nmap"
	}

	if _, err := exec.LookPath(path); err != nil {
		return nil, ErrNotInstalled
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	// -Pn skips host discovery, which a firewall hides anyway, -n the
	// reverse lookups, --open keeps the report to what matters.
	args := []string{"-Pn", "-n", "-T4", "--open", "--top-ports", strconv.Itoa(min(max(topPorts, 1), MaxTopPorts)), "-oX", "-"}
	if addr.Unmap().Is6() {
		args = append(args, "-6")
	}

	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, append(args, addr.Unmap().String())...) //nolint:gosec // the path is the operator's, the address validated
	cmd.Stdout, cmd.Stderr = &out, &stderr

	// A child holding the pipe open must not outlive the timeout.
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("nmap: %w", ctx.Err())
		}

		return nil, fmt.Errorf("nmap: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	report, err := nmapreport.Parse(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("nmap: %w", err)
	}

	var ports []Port
	for _, h := range report.Hosts {
		for _, p := range h.Open() {
			ports = append(ports, Port{Number: p.Number, Protocol: p.Protocol, State: p.State,
				Service: p.Service, Banner: p.Banner()})
		}
	}

	return ports, nil
}
