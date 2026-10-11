// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/yousysadmin/gosint/internal/domain/user"
)

// TestMain hashes passwords at the lowest cost: at the real one every
// account the tests make costs seconds under the race detector.
func TestMain(m *testing.M) {
	user.BcryptCost = bcrypt.MinCost

	os.Exit(m.Run())
}
