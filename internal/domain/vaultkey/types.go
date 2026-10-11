// Gosint, Copyright (c) 2026 YouSysAdmin

package vaultkey

import keymodel "github.com/yousysadmin/gosint/internal/models/vaultkey"

// putInput stores a key, replacing one of the same name.
type putInput struct {
	Name  string `json:"name"  validate:"required,keyname" normalize:"trim"`
	Value string `json:"value" validate:"required,notblank,max=8192" normalize:"trim"`
}

// keyResponse answers a stored key, never its value.
type keyResponse struct {
	Key keymodel.Key `json:"key"`
}

// keysResponse answers the list.
type keysResponse struct {
	Keys []keymodel.Key `json:"keys"`
}
