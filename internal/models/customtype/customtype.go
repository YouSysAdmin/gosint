// Gosint, Copyright (c) 2026 YouSysAdmin

// Package customtype is an entity type declared as data by an
// administrator.
package customtype

import (
	"time"

	"github.com/yousysadmin/gosint/internal/core/types"
)

// CustomType is the stored declaration.
type CustomType struct {
	ID        string    `json:"id"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	types.CustomType
}
