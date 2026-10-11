// Gosint, Copyright (c) 2026 YouSysAdmin

package user

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// Store persists accounts. The address is a keyed hash in email with
// its sealed copy, the password hash, the names and the avatar sealed.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the user store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

// emailPurpose is the keyed hash an address is matched by.
const emailPurpose = "gosint/email"

// EmailIndex is what users.email holds for an address. Invitations
// match addresses the same way.
func EmailIndex(codec database.Codec, email string) (string, error) {
	return codec.Index(emailPurpose, strings.ToLower(strings.TrimSpace(email)))
}

func aad(column, id string) string { return database.AAD("users", column, id) }

const userSelect = `
SELECT id, email, email_sealed, password_hash, first_name, last_name, avatar_url,
       is_active, created_at, updated_at, admin, account_type,
       totp_secret, totp_enabled, totp_last_step, totp_failures, totp_locked_until,
       login_failures, login_locked_until
FROM users`

const userInsert = `
		INSERT INTO users (id, email, email_sealed, password_hash, first_name, last_name, avatar_url,
		                   is_active, created_at, updated_at, admin, account_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func (s *Store) insertArgs(u *usermodel.User) ([]any, error) {
	if u.AccountType == 0 {
		u.AccountType = usermodel.AccountLocal
	}

	c, err := s.sealed(u)
	if err != nil {
		return nil, err
	}

	return []any{u.ID, c[0], c[1], c[2], c[3], c[4], c[5],
		u.Active, u.CreatedAt, u.UpdatedAt, u.Admin, u.AccountType}, nil
}

// sealed is u's email index, sealed email, password hash, names and
// avatar, as stored.
func (s *Store) sealed(u *usermodel.User) ([6]string, error) {
	var (
		c    [6]string
		errs [6]error
	)

	c[0], errs[0] = EmailIndex(s.codec, u.Email)
	for i, f := range []struct{ col, v string }{
		{"email_sealed", u.Email}, {"password_hash", u.PasswordHash},
		{"first_name", u.FirstName}, {"last_name", u.LastName}, {"avatar_url", u.AvatarURL},
	} {
		c[i+1], errs[i+1] = s.codec.SealText(aad(f.col, u.ID), f.v)
	}

	return c, errors.Join(errs[:]...)
}

func (s *Store) scanUser(row interface{ Scan(...any) error }) (*usermodel.User, error) {
	var (
		u           usermodel.User
		emailSealed string
	)

	var totpLocked, loginLocked sql.NullTime
	err := row.Scan(&u.ID, &u.Email, &emailSealed, &u.PasswordHash, &u.FirstName, &u.LastName, &u.AvatarURL,
		&u.Active, &u.CreatedAt, &u.UpdatedAt, &u.Admin, &u.AccountType,
		&u.TOTPSecret, &u.TOTPEnabled, &u.TOTPLastStep, &u.TOTPFailures, &totpLocked,
		&u.LoginFailures, &loginLocked)
	if err != nil {
		return nil, err
	}

	u.Email = emailSealed

	var errs [5]error
	for i, f := range []struct {
		col string
		v   *string
	}{
		{"email_sealed", &u.Email}, {"password_hash", &u.PasswordHash},
		{"first_name", &u.FirstName}, {"last_name", &u.LastName}, {"avatar_url", &u.AvatarURL},
	} {
		*f.v, errs[i] = s.codec.OpenText(aad(f.col, u.ID), *f.v)
	}

	if err := errors.Join(errs[:]...); err != nil {
		return nil, err
	}

	if totpLocked.Valid {
		u.TOTPLockedUntil = &totpLocked.Time
	}

	if loginLocked.Valid {
		u.LoginLockedUntil = &loginLocked.Time
	}

	return &u, nil
}

func (s *Store) one(row *sql.Row) (*usermodel.User, error) {
	u, err := s.scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return u, err
}

// Create inserts u.
func (s *Store) Create(ctx context.Context, u *usermodel.User) error {
	args, err := s.insertArgs(u)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, userInsert, args...)

	return err
}

// Get returns one account by id.
func (s *Store) Get(ctx context.Context, id string) (*usermodel.User, error) {
	return s.one(s.QueryRow(ctx, userSelect+` WHERE id = ?`, id))
}

// CreateFirst inserts u as the administrator while the install is
// unclaimed. The claim is a one-row table written in the same
// transaction, so a second registration racing the first finds the row
// taken (on postgres it waits on the key until the first commits) and
// creates nothing, on both dialects.
func (s *Store) CreateFirst(ctx context.Context, u *usermodel.User) (bool, error) {
	created := false
	err := s.Tx(ctx, func(tx *database.Tx) error {
		// An account made another way (an identity provider's) has
		// claimed the install as surely as the row.
		var n int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return err
		}

		if n > 0 {
			return nil
		}

		res, err := tx.Exec(ctx, `INSERT INTO install_claim (id) VALUES (1) ON CONFLICT (id) DO NOTHING`)
		if err != nil {
			return err
		}

		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return err
		}

		u.Admin = true
		args, err := s.insertArgs(u)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, userInsert, args...)
		created = err == nil

		return err
	})

	return created, err
}

// GetMany returns the accounts among ids, in one query.
func (s *Store) GetMany(ctx context.Context, ids []string) ([]usermodel.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	return s.list(ctx, userSelect+` WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")+`)`, args...)
}

// GetByEmail returns one account by its (lowercased) email, matched by
// its keyed hash.
func (s *Store) GetByEmail(ctx context.Context, email string) (*usermodel.User, error) {
	idx, err := EmailIndex(s.codec, email)
	if err != nil {
		return nil, err
	}

	return s.one(s.QueryRow(ctx, userSelect+` WHERE email = ?`, idx))
}

// Count is how many accounts exist.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)

	return n, err
}

// UpdateProfile writes the self-editable fields.
func (s *Store) UpdateProfile(ctx context.Context, u *usermodel.User) error {
	c, err := s.sealed(u)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		UPDATE users SET first_name = ?, last_name = ?, avatar_url = ?, updated_at = ?
		WHERE id = ?`,
		c[3], c[4], c[5], u.UpdatedAt, u.ID)

	return err
}

// SearchColleagues lists active accounts whose email starts with prefix
// and who share at least one investigation with callerID, the caller
// left out. Strangers are found by their full address only.
// The addresses are sealed, so the colleagues are read and the prefix
// checked here: a caller's colleagues are a handful.
func (s *Store) SearchColleagues(ctx context.Context, callerID, prefix string, limit int) ([]usermodel.User, error) {
	all, err := s.list(ctx, userSelect+`
		WHERE id <> ? AND is_active = ?
		  AND EXISTS (
		    SELECT 1 FROM investigation_members theirs
		    JOIN investigation_members mine ON mine.investigation_id = theirs.investigation_id
		    WHERE theirs.user_id = users.id AND mine.user_id = ?)`,
		callerID, true, callerID)
	if err != nil {
		return nil, err
	}

	prefix = strings.ToLower(prefix)
	out := []usermodel.User{}
	for _, u := range all {
		if strings.HasPrefix(strings.ToLower(u.Email), prefix) {
			out = append(out, u)
		}
	}

	slices.SortFunc(out, func(a, b usermodel.User) int { return strings.Compare(a.Email, b.Email) })

	return out[:min(len(out), limit)], nil
}

func (s *Store) list(ctx context.Context, query string, args ...any) ([]usermodel.User, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []usermodel.User
	for rows.Next() {
		u, err := s.scanUser(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *u)
	}

	return out, rows.Err()
}

// Find lists accounts for the admin panel, newest first, with the total
// before the limit.
// The addresses are sealed, so a search reads the accounts the flags
// leave and looks for the piece here.
func (s *Store) Find(ctx context.Context, f UserFilter) ([]usermodel.User, int, error) {
	where, args := ` WHERE 1 = 1`, []any{}

	if f.Admin != nil {
		where += ` AND admin = ?`
		args = append(args, *f.Admin)
	}

	if f.Disabled != nil {
		where += ` AND is_active = ?`
		args = append(args, !*f.Disabled)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	if f.Query == "" {
		var total int
		if err := s.QueryRow(ctx, `SELECT COUNT(*) FROM users`+where, args...).Scan(&total); err != nil {
			return nil, 0, err
		}

		users, err := s.list(ctx, userSelect+where+` ORDER BY created_at DESC, id LIMIT ?`, append(args, limit)...)

		return users, total, err
	}

	all, err := s.list(ctx, userSelect+where+` ORDER BY created_at DESC, id`, args...)
	if err != nil {
		return nil, 0, err
	}

	q := strings.ToLower(f.Query)
	found := []usermodel.User{}
	for _, u := range all {
		if strings.Contains(strings.ToLower(u.Email), q) {
			found = append(found, u)
		}
	}

	return found[:min(len(found), limit)], len(found), nil
}

// UpdateFlags sets admin and active. With keep false the write is
// conditional on another enabled administrator existing, so the last
// one cannot be demoted or disabled. The statement runs in a
// serializable transaction: on postgres two demotions of the last two
// administrators would otherwise each see the other still in place.
// Reports whether the row changed.
func (s *Store) UpdateFlags(ctx context.Context, id string, admin, active, keep bool, now time.Time) (bool, error) {
	var changed bool
	err := s.SerializableTx(ctx, func(tx *database.Tx) error {
		res, err := tx.Exec(ctx, `
			UPDATE users SET admin = ?, is_active = ?, updated_at = ?
			WHERE id = ? AND (? OR (SELECT COUNT(*) FROM users u2 WHERE u2.admin = ? AND u2.is_active = ? AND u2.id <> ?) > 0)`,
			admin, active, now, id, keep, true, true, id)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		changed = n > 0

		return err
	})

	return changed, err
}

// DeleteKeepingAnAdmin removes the account unless it is the last
// enabled administrator, serializable like UpdateFlags. Reports whether
// the row went.
func (s *Store) DeleteKeepingAnAdmin(ctx context.Context, id string) (bool, error) {
	var gone bool
	err := s.SerializableTx(ctx, func(tx *database.Tx) error {
		res, err := tx.Exec(ctx, `
			DELETE FROM users
			WHERE id = ? AND (NOT (admin = ? AND is_active = ?)
				OR (SELECT COUNT(*) FROM users u2 WHERE u2.admin = ? AND u2.is_active = ? AND u2.id <> ?) > 0)`,
			id, true, true, true, true, id)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		gone = n > 0

		return err
	})

	return gone, err
}

// SetPassword writes a new hash. Only the hash: a whole row written
// from a read taken before the request would undo what an
// administrator changed in between.
func (s *Store) SetPassword(ctx context.Context, id, hash string, now time.Time) error {
	sealed, err := s.codec.SealText(aad("password_hash", id), hash)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `UPDATE users SET password_hash = ?, account_type = ?, updated_at = ? WHERE id = ?`,
		sealed, usermodel.AccountLocal, now, id)

	return err
}

// SetTOTP writes the sealed secret and whether it is enabled.
func (s *Store) SetTOTP(ctx context.Context, id, secret string, enabled bool) error {
	_, err := s.Exec(ctx, `UPDATE users SET totp_secret = ?, totp_enabled = ? WHERE id = ?`, secret, enabled, id)

	return err
}

// ClaimTOTPStep records step as spent and reports whether this call
// did it. The guard is the WHERE clause, so two requests presenting
// one code at the same instant cannot both pass, and an earlier step
// within the skew window is refused too.
func (s *Store) ClaimTOTPStep(ctx context.Context, id string, step int64) (bool, error) {
	res, err := s.Exec(ctx, `UPDATE users SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?`, step, id, step)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// LockedUntil reads one of the two locks, "totp" or "login".
func (s *Store) LockedUntil(ctx context.Context, id, which string) (*time.Time, error) {
	column := "login_locked_until"
	if which == "totp" {
		column = "totp_locked_until"
	}

	var until sql.NullTime
	err := s.QueryRow(ctx, `SELECT `+column+` FROM users WHERE id = ?`, id).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !until.Valid {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &until.Time, nil
}

// RecordFailure counts one wrong password ("login") or code ("totp")
// and, at limit, locks until the time given and clears the count.
// Reports whether the lock is now held.
func (s *Store) RecordFailure(ctx context.Context, id, which string, limit int, until time.Time) (bool, error) {
	count, lock := "login_failures", "login_locked_until"
	if which == "totp" {
		count, lock = "totp_failures", "totp_locked_until"
	}

	_, err := s.Exec(ctx, `
		UPDATE users SET
			`+lock+` = CASE WHEN `+count+` + 1 >= ? THEN ? ELSE `+lock+` END,
			`+count+` = CASE WHEN `+count+` + 1 >= ? THEN 0 ELSE `+count+` + 1 END
		WHERE id = ?`, limit, until, limit, id)
	if err != nil {
		return false, err
	}

	held, err := s.LockedUntil(ctx, id, which)
	if err != nil {
		return false, err
	}

	return held != nil && held.After(time.Now()), nil
}

// ClearFailures forgets the count and the lock after a right answer.
func (s *Store) ClearFailures(ctx context.Context, id, which string) error {
	count, lock := "login_failures", "login_locked_until"
	if which == "totp" {
		count, lock = "totp_failures", "totp_locked_until"
	}

	_, err := s.Exec(ctx, `UPDATE users SET `+count+` = 0, `+lock+` = NULL WHERE id = ?`, id)

	return err
}

// ReplaceRecoveryCodes stores a fresh set, dropping whatever the account
// had, in one transaction.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, id string, hashes []string, now time.Time) error {
	return s.Tx(ctx, func(tx *database.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, id); err != nil {
			return err
		}

		for _, h := range hashes {
			if _, err := tx.Exec(ctx, `INSERT INTO user_recovery_codes (id, user_id, code_hash, created_at) VALUES (?, ?, ?, ?)`,
				ids.New(), id, h, now); err != nil {
				return err
			}
		}

		return nil
	})
}

// ClaimRecoveryCode spends the code whose hash matches, once.
func (s *Store) ClaimRecoveryCode(ctx context.Context, id, hash string, now time.Time) (bool, error) {
	res, err := s.Exec(ctx, `
		UPDATE user_recovery_codes SET used_at = ?
		WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`, now, id, hash)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// RemainingRecoveryCodes counts the unspent codes.
func (s *Store) RemainingRecoveryCodes(ctx context.Context, id string) (int, error) {
	var n int
	err := s.QueryRow(ctx, `SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL`, id).Scan(&n)

	return n, err
}

// DeleteRecoveryCodes removes every code the account has.
func (s *Store) DeleteRecoveryCodes(ctx context.Context, id string) error {
	_, err := s.Exec(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, id)

	return err
}
