// Gosint, Copyright (c) 2026 YouSysAdmin

// Package subfinder runs ProjectDiscovery's subfinder as a library.
package subfinder

import (
	"context"
	"io"
	"maps"
	"slices"
	"sync"

	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/levels"
	"github.com/projectdiscovery/subfinder/v2/pkg/runner"
)

// quiet silences subfinder's own logger once, which otherwise writes
// progress to stderr beside gosint's log.
var quiet = sync.OnceFunc(func() {
	gologger.DefaultLogger.SetMaxLevel(levels.LevelSilent)
})

// Enumerate runs subfinder's default passive sources.
//
// Sources needing keys use the operator's own subfinder provider config
// (~/.config/subfinder/provider-config.yaml, or SUBFINDER_PROVIDER_CONFIG)
// when there is one, which is read and never written.
func Enumerate(ctx context.Context, domain string) ([]string, error) {
	quiet()

	r, err := runner.NewRunner(&runner.Options{
		Silent:             true,
		Threads:            10,
		Timeout:            30,
		MaxEnumerationTime: 5,
		Output:             io.Discard,
		DisableUpdateCheck: true,
	})
	if err != nil {
		return nil, err
	}

	found, err := r.EnumerateSingleDomainWithCtx(ctx, domain, []io.Writer{io.Discard})
	if err != nil {
		return nil, err
	}

	return slices.Sorted(maps.Keys(found)), nil
}
