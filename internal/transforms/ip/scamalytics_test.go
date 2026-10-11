// Gosint, Copyright (c) 2026 YouSysAdmin

package ip

import (
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
)

type discard struct{}

func (discard) Log(logmodel.Level, string, map[string]any) {}

// The API host prefix is one DNS label: anything else is refused before
// a request is built from it.
func TestTheScamalyticsHostIsOneLabel(t *testing.T) {
	for host, ok := range map[string]bool{"api12": true, "evil.example/x?": false, "a@b": false, "": false} {
		x := transform.NewExec(transform.ExecConfig{Log: discard{}, Types: types.Builtin(),
			Params: map[string]string{"SCAMLYTICS_API_HOST": host}})
		err := IPToFraudScore{}.Run(t.Context(), x, []graphmodel.Node{})
		if refused := err != nil && strings.Contains(err.Error(), "host prefix"); refused == ok {
			t.Errorf("host %q: err = %v", host, err)
		}
	}
}
