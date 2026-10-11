// Gosint, Copyright (c) 2026 YouSysAdmin

// Package store aggregates the per-domain persistence interfaces
// behind a single struct. Handlers depend on these, never on a
// concrete store.
//
// A Get that finds nothing returns (nil, nil).
package store

import (
	"context"
	"time"

	"github.com/yousysadmin/gosint/internal/models/analysis"
	"github.com/yousysadmin/gosint/internal/models/attachment"
	"github.com/yousysadmin/gosint/internal/models/chat"

	"github.com/yousysadmin/gosint/internal/models/customtype"
	"github.com/yousysadmin/gosint/internal/models/flow"
	"github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/models/investigation"
	"github.com/yousysadmin/gosint/internal/models/llmmodel"
	"github.com/yousysadmin/gosint/internal/models/logentry"
	"github.com/yousysadmin/gosint/internal/models/oauthprovider"
	"github.com/yousysadmin/gosint/internal/models/passkey"
	"github.com/yousysadmin/gosint/internal/models/passwordreset"
	"github.com/yousysadmin/gosint/internal/models/scan"
	"github.com/yousysadmin/gosint/internal/models/schedule"
	"github.com/yousysadmin/gosint/internal/models/session"
	"github.com/yousysadmin/gosint/internal/models/sketch"
	"github.com/yousysadmin/gosint/internal/models/transformtemplate"
	"github.com/yousysadmin/gosint/internal/models/user"
	"github.com/yousysadmin/gosint/internal/models/vaultkey"
)

// Store bundles every per-domain store.
type Store struct {
	User          UserStore
	Session       SessionStore
	Investigation InvestigationStore
	Sketch        SketchStore
	Graph         GraphStore
	VaultKey      VaultKeyStore
	Scan          ScanStore
	Log           LogStore
	Template      TemplateStore
	Flow          FlowStore
	Chat          ChatStore
	Analysis      AnalysisStore
	Attachment    AttachmentStore
	PasswordReset PasswordResetStore
	Passkey       PasskeyStore
	OAuthProvider OAuthProviderStore
	OAuthIdentity OAuthIdentityStore
	Schedule      ScheduleStore
	CustomType    CustomTypeStore
	Setting       SettingStore
	LLMModel      LLMModelStore
}

// SettingStore persists the install-wide settings, each one sealed
// JSON value by name.
type SettingStore interface {
	Get(ctx context.Context, name string, dst any) (bool, error)
	Put(ctx context.Context, name string, v any, by string) error
}

// LLMModelStore persists the language models an administrator offers.
type LLMModelStore interface {
	List(ctx context.Context) ([]llmmodel.Model, error)
	Get(ctx context.Context, id string) (*llmmodel.Model, error)
	Put(ctx context.Context, m *llmmodel.Model) error
	Delete(ctx context.Context, id string) (bool, error)
}

// CustomTypeStore persists the entity types declared as data.
type CustomTypeStore interface {
	List(ctx context.Context) ([]customtype.CustomType, error)
	Get(ctx context.Context, id string) (*customtype.CustomType, error)
	Create(ctx context.Context, t *customtype.CustomType) error
	Update(ctx context.Context, t *customtype.CustomType) error
	Delete(ctx context.Context, id string) (bool, error)

	// CountNodes is how many entities of the type exist anywhere.
	CountNodes(ctx context.Context, typeName string) (int, error)
}

// ScheduleStore persists the flows run again every so often. Claim is
// the one-node-only step before a run is queued.
type ScheduleStore interface {
	Create(ctx context.Context, s *schedule.Schedule) error
	Get(ctx context.Context, sketchID, id string) (*schedule.Schedule, error)
	ListBySketch(ctx context.Context, sketchID string) ([]schedule.Schedule, error)
	ListEnabled(ctx context.Context) ([]schedule.Schedule, error)
	Update(ctx context.Context, s *schedule.Schedule) error
	Delete(ctx context.Context, sketchID, id string) (bool, error)
	Claim(ctx context.Context, s *schedule.Schedule, scanID string, now time.Time) (bool, error)
	Fail(ctx context.Context, id, reason string, now time.Time) error
}

// OAuthProviderStore persists the identity providers, the client
// secret sealed at rest and plain in the record.
type OAuthProviderStore interface {
	Get(ctx context.Context, id string) (*oauthprovider.Provider, error)
	GetBySlug(ctx context.Context, slug string) (*oauthprovider.Provider, error)
	List(ctx context.Context) ([]oauthprovider.Provider, error)

	// ListLoginable is what the sign-in page offers: enabled, visible
	// and complete.
	ListLoginable(ctx context.Context) ([]oauthprovider.Provider, error)
	Put(ctx context.Context, p *oauthprovider.Provider) error
	Delete(ctx context.Context, id string) (bool, error)
	SlugTaken(ctx context.Context, slug, exceptID string) (bool, error)
}

// OAuthIdentityStore persists the links between external identities
// and accounts.
type OAuthIdentityStore interface {
	GetBySubject(ctx context.Context, providerID, subject string) (*oauthprovider.Identity, error)
	CountForUser(ctx context.Context, userID string) (int, error)
	ListForUser(ctx context.Context, userID string) ([]oauthprovider.Identity, error)

	// DeleteForUser unlinks one of the account's identities.
	DeleteForUser(ctx context.Context, id, userID string) (bool, error)

	// Link records the identity or refreshes it, one statement.
	Link(ctx context.Context, id *oauthprovider.Identity) error
}

// PasswordResetStore persists the single-use reset tokens, by hash.
type PasswordResetStore interface {
	GetByHash(ctx context.Context, hash string) (*passwordreset.Token, error)
	Put(ctx context.Context, t *passwordreset.Token) error

	// MarkUsed burns a token and reports whether this call did it.
	MarkUsed(ctx context.Context, id string, at time.Time) (bool, error)
	InvalidateForUser(ctx context.Context, userID string, at time.Time) error

	// RecentRequestIPs lists who asked for the user's links since a
	// moment, for the mail budget.
	RecentRequestIPs(ctx context.Context, userID string, since time.Time) ([]string, error)
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// PasskeyStore persists WebAuthn credentials and spent challenges.
type PasskeyStore interface {
	ListForUser(ctx context.Context, userID string) ([]passkey.Passkey, error)

	// GetByCredential resolves an asserted credential id, not scoped by
	// user since the credential is the identity claim.
	GetByCredential(ctx context.Context, credentialID string) (*passkey.Passkey, error)
	Put(ctx context.Context, p *passkey.Passkey) error
	RecordUse(ctx context.Context, id, credential string, at time.Time) error
	Rename(ctx context.Context, userID, id, name string) (bool, error)
	Delete(ctx context.Context, userID, id string) (bool, error)
	DeleteAllForUser(ctx context.Context, userID string) (int, error)
	CountForUser(ctx context.Context, userID string) (int, error)

	// SpendChallenge records a finished ceremony and reports whether it
	// was the first finish, pruning rows expired by now.
	SpendChallenge(ctx context.Context, challenge string, expiresAt, now time.Time) (bool, error)
}

// AttachmentStore persists the records of files attached to entities.
// The bytes are in blob.Store under Hash.
type AttachmentStore interface {
	Create(ctx context.Context, a *attachment.Attachment) error
	Get(ctx context.Context, sketchID, id string) (*attachment.Attachment, error)
	ListForNode(ctx context.Context, sketchID, nodeID string) ([]attachment.Attachment, error)
	ListForSketch(ctx context.Context, sketchID string) ([]attachment.Attachment, error)
	Delete(ctx context.Context, sketchID, id string) (bool, error)

	// HashesOfNodes and HashesOfSketch list the blobs the files name,
	// read before a delete so the blobs can be released after it.
	HashesOfNodes(ctx context.Context, sketchID string, nodeIDs []string) ([]string, error)
	HashesOfSketch(ctx context.Context, sketchID string) ([]string, error)

	// CountByHash is how many records across every sketch name the blob
	// of the sha256, CountByKey of the keyed hash the records hold.
	CountByHash(ctx context.Context, hash string) (int, error)
	CountByKey(ctx context.Context, key string) (int, error)

	// ListForNodes and Restore serve the undo journal: the records of
	// deleted nodes travel in the snapshot and come back with them.
	ListForNodes(ctx context.Context, sketchID string, nodeIDs []string) ([]attachment.Attachment, error)
	Restore(ctx context.Context, a attachment.Attachment) error

	// Blobs no record names are kept for a while: MarkReleased notes
	// the moment, ReleasedBefore lists what the sweep may delete, Unmark
	// forgets one.
	MarkReleased(ctx context.Context, hashes []string, now time.Time) error
	ReleasedBefore(ctx context.Context, before time.Time) ([]string, error)
	Unmark(ctx context.Context, key string) error
}

// AnalysisStore persists the written analyses of an investigation.
type AnalysisStore interface {
	Create(ctx context.Context, a *analysis.Analysis) error
	Update(ctx context.Context, a *analysis.Analysis) error
	Get(ctx context.Context, investigationID, id string) (*analysis.Analysis, error)

	// List leaves the content out.
	List(ctx context.Context, investigationID string) ([]analysis.Analysis, error)
	Delete(ctx context.Context, investigationID, id string) (bool, error)
}

// ChatStore persists conversations with the assistant, each an
// account's own about one sketch.
type ChatStore interface {
	Create(ctx context.Context, c *chat.Chat) error
	Get(ctx context.Context, sketchID, ownerID, id string) (*chat.Chat, error)
	List(ctx context.Context, sketchID, ownerID string) ([]chat.Chat, error)

	// Delete removes the chat with its turns, false when there was none.
	Delete(ctx context.Context, sketchID, ownerID, id string) (bool, error)

	// Touch records activity and, when title is not empty, a title.
	Touch(ctx context.Context, id, title string, at time.Time) error
	AddMessage(ctx context.Context, m *chat.Message) error

	// Messages returns the last limit turns, oldest first.
	Messages(ctx context.Context, chatID string, limit int) ([]chat.Message, error)
}

// FlowStore persists flows. Every method is scoped to the owner.
type FlowStore interface {
	Create(ctx context.Context, f *flow.Flow) error
	Update(ctx context.Context, f *flow.Flow) error
	Get(ctx context.Context, ownerID, id string) (*flow.Flow, error)

	// List lists the owner's flows, those starting from inputType when it
	// is not "".
	List(ctx context.Context, ownerID, inputType string) ([]flow.Flow, error)
	Delete(ctx context.Context, ownerID, id string) (bool, error)
}

// TemplateStore persists YAML transforms. An account sees its own and
// the public ones.
type TemplateStore interface {
	Create(ctx context.Context, t *transformtemplate.Template) error
	Update(ctx context.Context, t *transformtemplate.Template) error

	// Get returns a template visible to userID: theirs, or public.
	Get(ctx context.Context, userID, id string) (*transformtemplate.Template, error)

	// Visible lists userID's templates and the public ones, by name.
	Visible(ctx context.Context, userID string) ([]transformtemplate.Template, error)

	// Delete removes one of the owner's templates.
	Delete(ctx context.Context, ownerID, id string) (bool, error)
}

// VaultKeyStore persists sealed API keys. Every method is scoped to the
// owning account.
type VaultKeyStore interface {
	// Put stores k, replacing the owner's key of the same name.
	Put(ctx context.Context, k *vaultkey.Key) error
	List(ctx context.Context, ownerID string) ([]vaultkey.Key, error)
	Get(ctx context.Context, ownerID, id string) (*vaultkey.Key, error)
	GetByName(ctx context.Context, ownerID, name string) (*vaultkey.Key, error)
	Delete(ctx context.Context, ownerID, id string) (bool, error)
}

// ScanStore persists scans, and is the job queue workers claim from.
type ScanStore interface {
	Enqueue(ctx context.Context, s *scan.Scan) error
	Get(ctx context.Context, id string) (*scan.Scan, error)

	// ListBySketch lists the sketch's newest scans first.
	ListBySketch(ctx context.Context, sketchID string, limit int) ([]scan.Scan, error)
	Delete(ctx context.Context, id string) error

	// Claim takes up to limit pending scans for worker, running and
	// leased until lease. Safe for several workers on several nodes.
	Claim(ctx context.Context, worker string, now, lease time.Time, limit int) ([]scan.Scan, error)

	// Renew extends the lease. False when the scan is no longer this
	// worker's to run, because it was cancelled or recovered.
	Renew(ctx context.Context, id, worker string, lease time.Time) (bool, error)

	// Finish settles a running scan. False when it is no longer this
	// worker's to settle.
	Finish(ctx context.Context, s *scan.Scan, worker string) (bool, error)

	// Release hands a running scan back to the queue, for a worker
	// shutting down mid-run.
	Release(ctx context.Context, id, worker string) (bool, error)

	// Cancel stops a scan that has not settled yet.
	Cancel(ctx context.Context, id string, now time.Time) (bool, error)

	// RecoverExpired puts back running scans whose lease ran out, or
	// fails them once they reached maxAttempts.
	RecoverExpired(ctx context.Context, now time.Time, maxAttempts int) (requeued, failed int64, err error)
}

// LogStore persists sketch activity logs.
type LogStore interface {
	Append(ctx context.Context, e *logentry.Entry) error

	// List returns up to limit of the newest entries after since, oldest
	// first, the order a log panel prints them in.
	List(ctx context.Context, sketchID string, since *time.Time, limit int) ([]logentry.Entry, error)
	DeleteBySketch(ctx context.Context, sketchID string) (int64, error)
}

// UserStore persists accounts.
type UserStore interface {
	Create(ctx context.Context, u *user.User) error

	// CreateFirst inserts u only while no account exists, in one
	// transaction, and reports whether it did.
	CreateFirst(ctx context.Context, u *user.User) (bool, error)
	Get(ctx context.Context, id string) (*user.User, error)

	// GetMany returns the accounts among ids, absent ones left out.
	GetMany(ctx context.Context, ids []string) ([]user.User, error)
	GetByEmail(ctx context.Context, email string) (*user.User, error)
	Count(ctx context.Context) (int, error)

	// UpdateProfile writes the self-editable fields only.
	UpdateProfile(ctx context.Context, u *user.User) error

	// SearchColleagues lists up to limit active accounts whose email
	// starts with prefix and who share an investigation with callerID.
	SearchColleagues(ctx context.Context, callerID, prefix string, limit int) ([]user.User, error)

	// SetPassword writes a new hash and makes the account local.
	SetPassword(ctx context.Context, id, hash string, now time.Time) error

	// The admin panel: a filtered list with its total, the flags written
	// unless that would leave no enabled administrator, a delete under
	// the same rule.
	Find(ctx context.Context, f user.Filter) ([]user.User, int, error)
	UpdateFlags(ctx context.Context, id string, admin, active, keep bool, now time.Time) (bool, error)
	DeleteKeepingAnAdmin(ctx context.Context, id string) (bool, error)

	// The second factor: the sealed secret and its state, and the time
	// step a code was last accepted for, claimed once.
	SetTOTP(ctx context.Context, id, secret string, enabled bool) error
	ClaimTOTPStep(ctx context.Context, id string, step int64) (bool, error)

	// The two locks, which is "login" or "totp": read, count a failure
	// toward the lock (held reported), forget after a right answer.
	LockedUntil(ctx context.Context, id, which string) (*time.Time, error)
	RecordFailure(ctx context.Context, id, which string, limit int, until time.Time) (bool, error)
	ClearFailures(ctx context.Context, id, which string) error

	// Recovery codes, stored hashed, each spent once.
	ReplaceRecoveryCodes(ctx context.Context, id string, hashes []string, now time.Time) error
	ClaimRecoveryCode(ctx context.Context, id, hash string, now time.Time) (bool, error)
	RemainingRecoveryCodes(ctx context.Context, id string) (int, error)
	DeleteRecoveryCodes(ctx context.Context, id string) error
}

// SessionStore persists sign-ins, keyed by the hash of the cookie.
type SessionStore interface {
	Create(ctx context.Context, s *session.Session) error

	// Lookup returns the session for tokenHash unless it expired by now.
	Lookup(ctx context.Context, tokenHash string, now time.Time) (*session.Session, error)
	Extend(ctx context.Context, id string, expiresAt time.Time) error
	Delete(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)

	// The account's sign-ins: listed, one ended, all but one ended, all
	// ended. A session is a row, so ending one is deleting it.
	ListForUser(ctx context.Context, userID string, now time.Time) ([]session.Session, error)
	DeleteByID(ctx context.Context, userID, id string) (bool, error)
	DeleteOthers(ctx context.Context, userID, keepID string) (int64, error)
	DeleteForUser(ctx context.Context, userID string) (int64, error)
}

// InvestigationStore persists investigations and their members.
type InvestigationStore interface {
	// Create inserts the investigation and its owner as the first member.
	Create(ctx context.Context, inv *investigation.Investigation) error
	Get(ctx context.Context, id string) (*investigation.Investigation, error)

	// ListForUser lists every investigation userID is a member of.
	ListForUser(ctx context.Context, userID string) ([]investigation.WithRole, error)
	Update(ctx context.Context, inv *investigation.Investigation) error
	Delete(ctx context.Context, id string) error

	// Role is userID's role on the investigation, "" for a non-member.
	Role(ctx context.Context, investigationID, userID string) (investigation.Role, error)
	Members(ctx context.Context, investigationID string) ([]investigation.Member, error)
	AddMember(ctx context.Context, m *investigation.Member) error

	// SetMemberRole and RemoveMember report whether the member existed.
	SetMemberRole(ctx context.Context, investigationID, userID string, role investigation.Role) (bool, error)
	RemoveMember(ctx context.Context, investigationID, userID string) (bool, error)

	// OwnedBy lists the investigations the user owns, TransferOwner hands
	// one to another member (false when they are none).
	OwnedBy(ctx context.Context, userID string) ([]investigation.Investigation, error)
	TransferOwner(ctx context.Context, investigationID, fromUserID, toUserID string, now time.Time) (bool, error)

	// Invitations by link: created, looked up by token hash, the open
	// ones listed, the open one of an address found for a registration,
	// revoked, accepted once.
	CreateInvitation(ctx context.Context, inv *investigation.Invitation) error
	InvitationByHash(ctx context.Context, hash string) (*investigation.Invitation, error)
	PendingInvitations(ctx context.Context, investigationID string, now time.Time) ([]investigation.Invitation, error)
	PendingInvitationFor(ctx context.Context, hash, email string, now time.Time) (*investigation.Invitation, error)
	DeleteInvitation(ctx context.Context, investigationID, id string) (bool, error)
	AcceptInvitation(ctx context.Context, id string, at time.Time) (bool, error)
}

// SketchStore persists sketches.
type SketchStore interface {
	Create(ctx context.Context, s *sketch.Sketch) error
	Get(ctx context.Context, id string) (*sketch.Sketch, error)
	ListByInvestigation(ctx context.Context, investigationID string) ([]sketch.Sketch, error)

	// ListForUser lists the sketches of every investigation userID is a
	// member of.
	ListForUser(ctx context.Context, userID string) ([]sketch.Sketch, error)
	Update(ctx context.Context, s *sketch.Sketch) error
	Delete(ctx context.Context, id string) error
}

// GraphStore persists the nodes and edges of sketches. Every method is
// scoped to one sketch, and every write bumps that sketch's updated_at.
type GraphStore interface {
	Graph(ctx context.Context, sketchID string) ([]graph.Node, []graph.Edge, error)
	Node(ctx context.Context, sketchID, id string) (*graph.Node, error)

	// NodesIn returns the nodes of the sketch among ids, absent ones
	// left out.
	NodesIn(ctx context.Context, sketchID string, ids []string) ([]graph.Node, error)

	// UpsertNodes creates each node or, when one of the same type and
	// key (the primary field) exists on the sketch, merges its
	// properties into that node.
	UpsertNodes(ctx context.Context, sketchID string, nodes []graph.Node) ([]graph.Node, error)

	// Import upserts nodes and the edges among them, by index, in one
	// transaction. created counts the edges written.
	Import(ctx context.Context, sketchID string, nodes []graph.Node, edges []graph.EdgeRef) (written []graph.Node, created int, err error)
	UpdateNode(ctx context.Context, sketchID, id string, patch graph.NodePatch) (*graph.Node, error)
	SetPositions(ctx context.Context, sketchID string, positions []graph.Position) (int, error)

	// DeleteNodes removes the nodes and every edge touching them.
	DeleteNodes(ctx context.Context, sketchID string, ids []string) (int, error)

	// Clear removes every node and edge of the sketch.
	Clear(ctx context.Context, sketchID string) (int, error)

	// Dependents lists the nodes found through ids, the pieces left
	// hanging only from them once they are gone.
	Dependents(ctx context.Context, sketchID string, ids []string) ([]string, error)

	// UpsertEdge creates the edge or merges properties into the existing
	// one with the same source, target and label. Nil when either end is
	// not a node of the sketch.
	UpsertEdge(ctx context.Context, sketchID string, e graph.Edge) (*graph.Edge, error)
	UpdateEdge(ctx context.Context, sketchID, id string, patch graph.EdgePatch) (*graph.Edge, error)
	DeleteEdges(ctx context.Context, sketchID string, ids []string) (int, error)

	// MergeNodes folds ids into one node: keepID when it is one of them,
	// otherwise into the upsert of into. Edges to the outside are moved
	// onto the result, edges among the merged nodes go with them. Nil
	// when one of ids is not a node of the sketch.
	MergeNodes(ctx context.Context, sketchID string, ids []string, keepID string, into graph.Node) (*graph.Node, error)

	// Neighbors returns the node, the nodes one edge away and those
	// edges. Nil nodes when id is not a node of the sketch.
	Neighbors(ctx context.Context, sketchID, id string) ([]graph.Node, []graph.Edge, error)

	// NodeByKey is the node with that type and key, nil when none.
	NodeByKey(ctx context.Context, sketchID, typ, key string) (*graph.Node, error)
	EdgesIn(ctx context.Context, sketchID string, ids []string) ([]graph.Edge, error)

	// EdgesTouching is every edge with an end among nodeIDs, each once.
	EdgesTouching(ctx context.Context, sketchID string, nodeIDs []string) ([]graph.Edge, error)
	EdgeByEnds(ctx context.Context, sketchID, source, target, label string) (*graph.Edge, error)

	// The journal of hand edits, for undo and redo. RecordChange stores
	// an edit as the account's newest on the sketch and drops its redo
	// stack, NextUndo and NextRedo find what to revert next, Revert
	// writes a snapshot back over another, SetUndone flips the mark.
	RecordChange(ctx context.Context, ch *graph.Change) error
	NextUndo(ctx context.Context, sketchID, userID string) (*graph.Change, error)
	NextRedo(ctx context.Context, sketchID, userID string) (*graph.Change, error)
	SetUndone(ctx context.Context, id string, undone bool) error
	Revert(ctx context.Context, sketchID string, from, to graph.Snapshot) error
	PruneChanges(ctx context.Context, before time.Time) (int64, error)

	// Search finds entities across the sketches the account may read.
	Search(ctx context.Context, userID string, q SearchQuery) ([]Hit, error)
}

// SearchQuery is what a search looks for: Q a piece of the label,
// Type and Key the entity exactly, ExcludeSketch a sketch left out,
// Investigation the one investigation to look in.
type SearchQuery struct {
	Q, Type, Key, ExcludeSketch, Investigation string
	Limit                                      int
}

// Hit is one entity found, with where it sits.
type Hit struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Label             string `json:"label"`
	SketchID          string `json:"sketch_id"`
	SketchTitle       string `json:"sketch_title"`
	InvestigationID   string `json:"investigation_id"`
	InvestigationName string `json:"investigation_name"`
}
