// Gosint, Copyright (c) 2026 YouSysAdmin

package backend

import (
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/analysis"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	"github.com/yousysadmin/gosint/internal/domain/auth"
	"github.com/yousysadmin/gosint/internal/domain/chat"
	"github.com/yousysadmin/gosint/internal/domain/customtype"
	"github.com/yousysadmin/gosint/internal/domain/eventlog"
	"github.com/yousysadmin/gosint/internal/domain/flow"
	"github.com/yousysadmin/gosint/internal/domain/graph"
	"github.com/yousysadmin/gosint/internal/domain/investigation"
	"github.com/yousysadmin/gosint/internal/domain/llmmodel"
	"github.com/yousysadmin/gosint/internal/domain/oauthprovider"
	"github.com/yousysadmin/gosint/internal/domain/passkey"
	"github.com/yousysadmin/gosint/internal/domain/passwordreset"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	"github.com/yousysadmin/gosint/internal/domain/schedule"
	"github.com/yousysadmin/gosint/internal/domain/setting"
	"github.com/yousysadmin/gosint/internal/domain/sketch"
	"github.com/yousysadmin/gosint/internal/domain/store"
	"github.com/yousysadmin/gosint/internal/domain/transformtemplate"
	"github.com/yousysadmin/gosint/internal/domain/user"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
)

// BindStore wires the per-domain stores into the aggregate Store that
// handlers depend on. Both backends share every store. v seals what is
// stored, a nil vault stores clear text, which only tests use.
func BindStore(db database.Database, v *vault.Vault) *store.Store {
	return &store.Store{
		User:          user.NewStore(db, v),
		Session:       auth.NewSessionStore(db, v),
		Investigation: investigation.NewStore(db, v),
		Sketch:        sketch.NewStore(db, v),
		Graph:         graph.NewStore(db, graph.TypeLabeler, v),
		VaultKey:      vaultkey.NewStore(db, v),
		Scan:          scan.NewStore(db, v),
		Log:           eventlog.NewStore(db, v),
		Template:      transformtemplate.NewStore(db, v),
		Flow:          flow.NewStore(db, v),
		Chat:          chat.NewStore(db, v),
		Analysis:      analysis.NewStore(db, v),
		Attachment:    attachment.NewStore(db, v),
		PasswordReset: passwordreset.NewStore(db, v),
		Passkey:       passkey.NewStore(db, v),
		OAuthProvider: oauthprovider.NewStore(db, v),
		OAuthIdentity: oauthprovider.NewIdentityStore(db, v),
		Schedule:      schedule.NewStore(db, v),
		CustomType:    customtype.NewStore(db, v),
		Setting:       setting.NewStore(db, v),
		LLMModel:      llmmodel.NewStore(db, v),
	}
}
