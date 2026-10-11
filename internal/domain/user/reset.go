// Gosint, Copyright (c) 2026 YouSysAdmin

package user

import (
	"context"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	prmodel "github.com/yousysadmin/gosint/internal/models/passwordreset"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// IssueReset mints a reset token for u, stores its hash and returns the
// link to hand over. No budget and no mail: the request endpoint spends
// the one and sends the other, the admin panel hands the link over.
func IssueReset(ctx context.Context, rt *env.Runtime, u *usermodel.User, requestIP string) (string, error) {
	plaintext, hash, err := prmodel.Generate()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	tok := &prmodel.Token{ID: ids.New(), UserID: u.ID, TokenHash: hash, ExpiresAt: now.Add(prmodel.TTL),
		CreatedAt: now, RequestIP: requestIP}
	if err := rt.Store.PasswordReset.Put(ctx, tok); err != nil {
		return "", err
	}

	return strings.TrimRight(rt.Config.Server.PublicURL, "/") + env.ConsolePath + "/reset-password?token=" + plaintext, nil
}
