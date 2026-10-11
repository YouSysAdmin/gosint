// Gosint, Copyright (c) 2026 YouSysAdmin

package chat_test

import (
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	chatmodel "github.com/yousysadmin/gosint/internal/models/chat"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// A chat belongs to one account about one sketch: the turns come back
// oldest first within the limit, another account sees nothing, and
// deleting the chat takes its turns.
func TestChatsAreTheAccountsOwn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st := backend.BindStore(db, nil)
		ctx := t.Context()
		now := time.Now().UTC()

		mk := func(email string) string {
			u := &usermodel.User{ID: ids.New(), Email: email, PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
			if err := st.User.Create(ctx, u); err != nil {
				t.Fatal(err)
			}

			return u.ID
		}

		alice, bob := mk(ids.New()+"@example.com"), mk(ids.New()+"@example.com")
		inv := &invmodel.Investigation{ID: ids.New(), Name: "case", Status: "active", OwnerID: alice, CreatedAt: now, UpdatedAt: now}
		if err := st.Investigation.Create(ctx, inv); err != nil {
			t.Fatal(err)
		}

		sk := &sketchmodel.Sketch{ID: ids.New(), InvestigationID: inv.ID, Title: "s", Status: "active", OwnerID: alice, CreatedAt: now, UpdatedAt: now}
		if err := st.Sketch.Create(ctx, sk); err != nil {
			t.Fatal(err)
		}

		ch := &chatmodel.Chat{ID: ids.New(), SketchID: sk.ID, OwnerID: alice, CreatedAt: now, UpdatedAt: now}
		if err := st.Chat.Create(ctx, ch); err != nil {
			t.Fatal(err)
		}

		for i, text := range []string{"first", "second", "third"} {
			m := &chatmodel.Message{ID: ids.New(), ChatID: ch.ID, Role: "user", Content: text,
				Context: []string{"n1"}, CreatedAt: now.Add(time.Duration(i) * time.Second)}
			if err := st.Chat.AddMessage(ctx, m); err != nil {
				t.Fatal(err)
			}
		}

		last, err := st.Chat.Messages(ctx, ch.ID, 2)
		if err != nil || len(last) != 2 || last[0].Content != "second" || last[1].Content != "third" || last[0].Context[0] != "n1" {
			t.Fatalf("last two turns = %+v, %v", last, err)
		}

		if err := st.Chat.Touch(ctx, ch.ID, "about acme", now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}

		got, err := st.Chat.Get(ctx, sk.ID, alice, ch.ID)
		if err != nil || got == nil || got.Title != "about acme" {
			t.Fatalf("after touch = %+v, %v", got, err)
		}

		if err := st.Chat.Touch(ctx, ch.ID, "", now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}

		if got, _ = st.Chat.Get(ctx, sk.ID, alice, ch.ID); got.Title != "about acme" {
			t.Errorf("an empty title replaced the title: %q", got.Title)
		}

		if other, _ := st.Chat.Get(ctx, sk.ID, bob, ch.ID); other != nil {
			t.Error("another account read the chat")
		}

		if list, _ := st.Chat.List(ctx, sk.ID, bob); len(list) != 0 {
			t.Error("another account listed the chat")
		}

		gone, err := st.Chat.Delete(ctx, sk.ID, alice, ch.ID)
		if err != nil || !gone {
			t.Fatalf("delete = %v, %v", gone, err)
		}

		if turns, _ := st.Chat.Messages(ctx, ch.ID, 10); len(turns) != 0 {
			t.Errorf("turns outlived their chat: %d", len(turns))
		}
	})
}
