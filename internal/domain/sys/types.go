// Gosint, Copyright (c) 2026 YouSysAdmin

package sys

// unsealKeyInput initializes or unseals the server.
type unsealKeyInput struct {
	UnsealKey string `json:"unseal_key" validate:"required,max=1024"`
}

// statusResponse says what the server needs before it serves.
type statusResponse struct {
	// Initialized is false until a master key is wrapped and stored.
	Initialized bool `json:"initialized"`
	Sealed      bool `json:"sealed"`

	MinUnsealKey int `json:"min_unseal_key"`
}
