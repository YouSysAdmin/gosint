// Gosint, Copyright (c) 2026 YouSysAdmin

// Package location holds the transforms that start from a postal address.
package location

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// NominatimURL is OpenStreetMap's public geocoder.
const NominatimURL = "https://nominatim.openstreetmap.org/search"

// NominatimPace is the gap between two requests, the public instance
// allows one per second.
const NominatimPace = time.Second

// nominatim spaces the requests of every run on this server, so two
// scans at once stay within the public instance's policy.
var nominatim pacer

// pacer hands out request slots gap apart.
type pacer struct {
	mu   sync.Mutex
	next time.Time
}

// wait blocks until the next slot, gap after the one handed out before.
func (p *pacer) wait(ctx context.Context, gap time.Duration) error {
	p.mu.Lock()
	at := time.Now()
	if p.next.After(at) {
		at = p.next
	}

	p.next = at.Add(gap)
	p.mu.Unlock()

	t := time.NewTimer(time.Until(at))
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// LocationToCoordinates geocodes a postal address, so it can be shown on
// the map.
type LocationToCoordinates struct {
	BaseURL string

	// Pace is the gap between two requests.
	Pace time.Duration
}

// Meta describes the transform.
func (LocationToCoordinates) Meta() transform.Meta {
	return transform.Meta{Name: "location_to_coordinates", Category: "Location", Input: "location", Output: "location",
		Description: "Find the latitude and longitude of an address (OpenStreetMap Nominatim)."}
}

// nominatimPlace is one search result. Coordinates come as strings.
type nominatimPlace struct {
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	DisplayName string `json:"display_name"`
}

// Run enriches each location in place. One already placed is skipped.
func (e LocationToCoordinates) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		if _, ok := in.Properties["latitude"].(float64); ok {
			continue
		}

		if err := nominatim.wait(ctx, e.Pace); err != nil {
			return err
		}

		place, err := e.search(ctx, x, in.Properties)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if place == nil {
			x.Warn("%s: no match", in.Label)

			continue
		}

		lat, errLat := strconv.ParseFloat(place.Lat, 64)
		lon, errLon := strconv.ParseFloat(place.Lon, 64)
		if errLat != nil || errLon != nil {
			x.Warn("%s: unreadable coordinates %q, %q", in.Label, place.Lat, place.Lon)

			continue
		}

		// x.Node merges, so the two coordinates are all that is written.
		placed := map[string]any{"latitude": lat, "longitude": lon}
		for _, f := range []string{"address", "city", "country", "zip"} {
			if v, ok := in.Properties[f]; ok {
				placed[f] = v
			}
		}

		if _, err := x.Node(ctx, "location", placed); err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		x.Found("placed %s at %.5f, %.5f (%s)", in.Label, lat, lon, place.DisplayName)
	}

	return nil
}

// search asks for the address by its parts, the most precise query, and
// falls back to the whole address as free text.
func (e LocationToCoordinates) search(ctx context.Context, x *transform.Exec, p map[string]any) (*nominatimPlace, error) {
	structured := url.Values{"format": {"jsonv2"}, "limit": {"1"}}
	for field, key := range map[string]string{"address": "street", "city": "city", "country": "country", "zip": "postalcode"} {
		if v := strings.TrimSpace(props.Str(p, field)); v != "" {
			structured.Set(key, v)
		}
	}

	var got []nominatimPlace
	if _, err := httpx.GetJSON(ctx, x, e.BaseURL+"?"+structured.Encode(), nil, &got); err != nil {
		return nil, err
	}

	if len(got) > 0 {
		return &got[0], nil
	}

	var parts []string
	for _, field := range []string{"address", "zip", "city", "country"} {
		if v := strings.TrimSpace(props.Str(p, field)); v != "" {
			parts = append(parts, v)
		}
	}

	free := url.Values{"format": {"jsonv2"}, "limit": {"1"}, "q": {strings.Join(parts, ", ")}}
	if _, err := httpx.GetJSON(ctx, x, e.BaseURL+"?"+free.Encode(), nil, &got); err != nil {
		return nil, err
	}

	if len(got) > 0 {
		return &got[0], nil
	}

	return nil, nil
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{LocationToCoordinates{BaseURL: NominatimURL, Pace: NominatimPace}}
}
