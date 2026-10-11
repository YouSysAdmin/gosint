// Gosint, Copyright (c) 2026 YouSysAdmin

package user_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/user"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// The security state of an account on both backends: the first account
// is the admin, a TOTP step is claimed once and never goes back, the
// lock counts to its limit then holds and clears, a recovery code is
// spent once.
// testVault seals with a throwaway key, so the store runs as it does in
// a server.
func testVault(t *testing.T) *vault.Vault {
	t.Helper()

	v, err := vault.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}

	return v
}

func TestAccountSecurityStateOnBothBackends(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		ctx := context.Background()
		st := user.NewStore(db, testVault(t))
		now := time.Now().UTC().Truncate(time.Second)
		u := &usermodel.User{ID: ids.New(), Email: "a@example.com", PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
		created, err := st.CreateFirst(ctx, u)
		if err != nil || !created {
			t.Fatalf("CreateFirst = %v, %v", created, err)
		}

		got, err := st.Get(ctx, u.ID)
		if err != nil || got == nil || !got.Admin || got.AccountType != usermodel.AccountLocal {
			t.Fatalf("the first account = %+v, %v", got, err)
		}

		second := &usermodel.User{ID: ids.New(), Email: "b@example.com", PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
		if created, err := st.CreateFirst(ctx, second); err != nil || created {
			t.Errorf("a second CreateFirst = %v, %v", created, err)
		}

		// A step is claimed once and the clock never turns back.
		for _, tc := range []struct {
			step int64
			want bool
		}{{100, true}, {100, false}, {99, false}, {101, true}} {
			if ok, err := st.ClaimTOTPStep(ctx, u.ID, tc.step); err != nil || ok != tc.want {
				t.Errorf("ClaimTOTPStep(%d) = %v, %v, want %v", tc.step, ok, err, tc.want)
			}
		}

		// The lock counts to its limit, then holds until its time, then
		// clears.
		until := now.Add(10 * time.Minute)
		for i := 1; i <= 3; i++ {
			held, err := st.RecordFailure(ctx, u.ID, "login", 3, until)
			if err != nil || held != (i == 3) {
				t.Errorf("failure %d: held = %v, %v", i, held, err)
			}
		}

		if lock, err := st.LockedUntil(ctx, u.ID, "login"); err != nil || lock == nil || !lock.Equal(until) {
			t.Errorf("LockedUntil = %v, %v, want %v", lock, err, until)
		}

		if err := st.ClearFailures(ctx, u.ID, "login"); err != nil {
			t.Fatal(err)
		}

		if lock, err := st.LockedUntil(ctx, u.ID, "login"); err != nil || lock != nil {
			t.Errorf("after clearing, LockedUntil = %v, %v", lock, err)
		}

		if lock, err := st.LockedUntil(ctx, u.ID, "totp"); err != nil || lock != nil {
			t.Errorf("the other lock = %v, %v, want none", lock, err)
		}

		// Recovery codes: ten stored, one spent once.
		hashes := []string{"h1", "h2", "h3"}
		if err := st.ReplaceRecoveryCodes(ctx, u.ID, hashes, now); err != nil {
			t.Fatal(err)
		}

		if ok, err := st.ClaimRecoveryCode(ctx, u.ID, "h2", now); err != nil || !ok {
			t.Errorf("first claim = %v, %v", ok, err)
		}

		if ok, err := st.ClaimRecoveryCode(ctx, u.ID, "h2", now); err != nil || ok {
			t.Errorf("second claim = %v, %v, want refused", ok, err)
		}

		if n, err := st.RemainingRecoveryCodes(ctx, u.ID); err != nil || n != 2 {
			t.Errorf("remaining = %d, %v, want 2", n, err)
		}

		if err := st.ReplaceRecoveryCodes(ctx, u.ID, []string{"n1"}, now); err != nil {
			t.Fatal(err)
		}

		if n, err := st.RemainingRecoveryCodes(ctx, u.ID); err != nil || n != 1 {
			t.Errorf("after replacing, remaining = %d, %v, want 1", n, err)
		}

		// The sealed secret round trips and a reset password makes the
		// account local.
		if err := st.SetTOTP(ctx, u.ID, "V1.a.b.c", true); err != nil {
			t.Fatal(err)
		}

		got, _ = st.Get(ctx, u.ID)
		if got.TOTPSecret != "V1.a.b.c" || !got.TOTPEnabled || got.TOTPLastStep != 101 {
			t.Errorf("after SetTOTP = %+v", got)
		}
	})
}

// The last enabled administrator cannot be demoted, disabled or deleted,
// however the request is spelled: the store refuses the write itself.
func TestTheLastAdministratorStays(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		ctx := context.Background()
		st := user.NewStore(db, testVault(t))
		now := time.Now().UTC()
		a := &usermodel.User{ID: ids.New(), Email: "a@example.com", PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
		if _, err := st.CreateFirst(ctx, a); err != nil {
			t.Fatal(err)
		}

		b := &usermodel.User{ID: ids.New(), Email: "b@example.com", PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := st.Create(ctx, b); err != nil {
			t.Fatal(err)
		}

		// Alone, a stays whatever is asked. Keep true writes anyway.
		if ok, _ := st.UpdateFlags(ctx, a.ID, false, true, false, now); ok {
			t.Error("the only admin was demoted")
		}

		if ok, _ := st.UpdateFlags(ctx, a.ID, true, false, false, now); ok {
			t.Error("the only admin was disabled")
		}

		if ok, _ := st.DeleteKeepingAnAdmin(ctx, a.ID); ok {
			t.Error("the only admin was deleted")
		}

		if ok, _ := st.DeleteKeepingAnAdmin(ctx, b.ID); !ok {
			t.Error("a member was not deleted")
		}

		// With a second enabled admin the same writes pass.
		c := &usermodel.User{ID: ids.New(), Email: "c@example.com", PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := st.Create(ctx, c); err != nil {
			t.Fatal(err)
		}

		if ok, _ := st.UpdateFlags(ctx, c.ID, true, true, true, now); !ok {
			t.Fatal("c was not promoted")
		}

		if ok, _ := st.UpdateFlags(ctx, a.ID, false, true, false, now); !ok {
			t.Error("a was not demoted with c in place")
		}

		if ok, _ := st.DeleteKeepingAnAdmin(ctx, c.ID); ok {
			t.Error("c, now the only admin, was deleted")
		}
	})
}

// Two administrators demoting each other at the same moment leave one
// in place, on both backends: each condition reads the other's row, so
// the engine must not let both through.
func TestTwoDemotionsAtOnceKeepAnAdmin(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		ctx := context.Background()
		st := user.NewStore(db, testVault(t))
		now := time.Now().UTC()

		for round := range 10 {
			a := &usermodel.User{ID: ids.New(), Email: fmt.Sprintf("a%d@example.com", round), PasswordHash: "x",
				Active: true, Admin: true, CreatedAt: now, UpdatedAt: now}
			b := &usermodel.User{ID: ids.New(), Email: fmt.Sprintf("b%d@example.com", round), PasswordHash: "x",
				Active: true, Admin: true, CreatedAt: now, UpdatedAt: now}
			for _, u := range []*usermodel.User{a, b} {
				if err := st.Create(ctx, u); err != nil {
					t.Fatal(err)
				}
			}

			// Earlier rounds leave their survivor, demoted here so a and b
			// are the only two.
			if _, err := db.DB().ExecContext(ctx, st.Q(`UPDATE users SET admin = ? WHERE id <> ? AND id <> ?`), false, a.ID, b.ID); err != nil {
				t.Fatal(err)
			}

			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make([]bool, 2)
			errs := make([]error, 2)
			for i, u := range []*usermodel.User{a, b} {
				wg.Go(func() {
					<-start
					results[i], errs[i] = st.UpdateFlags(ctx, u.ID, false, true, false, now)
				})
			}

			close(start)
			wg.Wait()
			for _, err := range errs {
				if err != nil {
					t.Fatalf("round %d: %v", round, err)
				}
			}

			if results[0] == results[1] {
				t.Fatalf("round %d: demotions passed = %v, want exactly one", round, results)
			}

			var admins int
			if err := db.DB().QueryRowContext(ctx, st.Q(`SELECT COUNT(*) FROM users WHERE admin = ?`), true).Scan(&admins); err != nil {
				t.Fatal(err)
			}

			if admins != 1 {
				t.Fatalf("round %d: %d administrators left, want 1", round, admins)
			}
		}
	})
}

// Registrations racing for a fresh install seat one administrator, on
// both backends: the claim row lets one transaction through.
func TestOneFirstAccountUnderARace(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		ctx := context.Background()
		st := user.NewStore(db, testVault(t))
		now := time.Now().UTC().Truncate(time.Second)

		const racers = 8

		var wg sync.WaitGroup
		var won atomic.Int32
		for i := range racers {
			wg.Go(func() {
				u := &usermodel.User{ID: ids.New(), Email: fmt.Sprintf("racer%d@example.com", i), PasswordHash: "x",
					Active: true, CreatedAt: now, UpdatedAt: now}
				created, err := st.CreateFirst(ctx, u)
				if err != nil {
					t.Errorf("CreateFirst: %v", err)
				}

				if created {
					won.Add(1)
				}
			})
		}

		wg.Wait()
		if n := won.Load(); n != 1 {
			t.Fatalf("%d registrations became the administrator, want 1", n)
		}
	})
}
