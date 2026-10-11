// Gosint, Copyright (c) 2026 YouSysAdmin

// Package crtsh queries certificate transparency logs through crt.sh.
package crtsh

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
)

// URL is the public instance.
const URL = "https://crt.sh/"

// Entry is one certificate as crt.sh lists it.
type Entry struct {
	NameValue string `json:"name_value"`
}

// Retries are the pauses before asking again after a 5xx, which crt.sh
// answers whenever its database is overloaded. A test shortens them.
var Retries = []time.Duration{5 * time.Second, 15 * time.Second}

// Query asks base for the certificates matching query, which may hold
// the % wildcard.
func Query(ctx context.Context, x *transform.Exec, base, query string) ([]Entry, error) {
	for attempt := 0; ; attempt++ {
		var entries []Entry
		_, err := httpx.GetJSONWithin(ctx, httpx.SlowTimeout, x, base+"?q="+url.QueryEscape(query)+"&output=json", nil, &entries)
		se, isStatus := errors.AsType[httpx.StatusError](err)
		if err == nil || !isStatus || se.Code < 500 || attempt >= len(Retries) {
			if isStatus && se.Code >= 500 {
				return nil, fmt.Errorf("%w, %d attempts, crt.sh is likely down, try again later", err, attempt+1)
			}

			return entries, err
		}

		x.Info("crt.sh answered %s, asking again in %s", se.Status, Retries[attempt])
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(Retries[attempt]):
		}
	}
}
