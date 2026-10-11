// Gosint, Copyright (c) 2026 YouSysAdmin

package chat

import (
	chatmodel "github.com/yousysadmin/gosint/internal/models/chat"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// createInput starts a chat. The title is optional, the first question
// becomes it otherwise.
type createInput struct {
	Title string `json:"title" validate:"max=200" normalize:"trim"`
}

// sendInput is one question, with the selected entities as context.
// WithChildren adds what was found through them: every entity an edge
// leads to from a selected one, and on from there.
type sendInput struct {
	Prompt       string   `json:"prompt"        validate:"required,notblank,max=20000" normalize:"trim"`
	NodeIDs      []string `json:"node_ids"      validate:"max=50,dive,uuid"`
	WithChildren bool     `json:"with_children"`

	// ModelID is the model the person picked, the default one when
	// empty or gone.
	ModelID string `json:"model_id" validate:"omitempty,uuid"`
}

// chatResponse answers one chat.
type chatResponse struct {
	Chat chatmodel.Chat `json:"chat"`
}

// chatsResponse lists chats.
type chatsResponse struct {
	Chats []chatmodel.Chat `json:"chats"`
}

// messagesResponse lists the turns of a chat.
type messagesResponse struct {
	Messages []chatmodel.Message `json:"messages"`
}

// tokenEvent is one piece of a streamed answer, or of the thinking
// before it on the thinking event.
type tokenEvent struct {
	Text string `json:"text"`
}

// doneEvent ends a stream with the stored answer.
type doneEvent struct {
	Message chatmodel.Message `json:"message"`
}

// errorEvent ends a stream that failed.
type errorEvent struct {
	Error string `json:"error"`
}

// applyInput is the changes the person accepted from a proposal.
type applyInput struct {
	Changes []Change `json:"changes" validate:"required,min=1,max=50,dive"`
}

// applyResponse says what was written, and how many runs were queued.
type applyResponse struct {
	Applied  int               `json:"applied"`
	Launched int               `json:"launched"`
	Nodes    []graphmodel.Node `json:"nodes"`
	Errors   []string          `json:"errors"`
}
