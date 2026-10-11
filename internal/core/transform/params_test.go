// Gosint, Copyright (c) 2026 YouSysAdmin

package transform_test

import (
	"testing"

	"github.com/yousysadmin/gosint/internal/core/transform"
)

// A number param is a plain decimal: NaN, Inf, an exponent or hex would
// reach a caller reading an int as zero.
func TestANumberParamIsPlainDecimal(t *testing.T) {
	params := []transform.Param{{Name: "n", Type: transform.ParamNumber}}
	for v, ok := range map[string]bool{"10": true, "-3": true, "2.5": true, "NaN": false, "Inf": false, "1e3": false, "0x10": false} {
		_, err := transform.ResolveParams(params, map[string]string{"n": v}, nil)
		if (err == nil) != ok {
			t.Errorf("%q: err = %v", v, err)
		}
	}
}
