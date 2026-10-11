// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// A sketch in somebody else's investigation answers exactly like a
// sketch that does not exist, so its id cannot be confirmed.
func TestAForeignSketchReadsLikeAMissingOne(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")
	sketch, _ := sketchFor(t, alice)

	foreignCode, foreign, _ := bob.raw("GET", sketch, nil)
	missingCode, missing, _ := bob.raw("GET", "/api/sketches/"+ids.New(), nil)

	if foreignCode != http.StatusNotFound || missingCode != http.StatusNotFound {
		t.Fatalf("foreign = %d, missing = %d, want 404 both", foreignCode, missingCode)
	}

	if foreign != missing {
		t.Errorf("foreign body %q differs from missing body %q", foreign, missing)
	}
}
