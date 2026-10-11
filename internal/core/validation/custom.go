// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// nodeTypeRe is an entity type name, lowercase.
var nodeTypeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// keyNameRe is a secret name, such as WHOXY_API_KEY.
var keyNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// registerCustom wires app-specific validators onto v. Each rule gets a
// short tag name matched by defaultMessage in errors.go.
func registerCustom(v *validator.Validate) {
	// notblank refuses a value that is only whitespace, which required
	// and min both let through.
	_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
		return strings.TrimSpace(fl.Field().String()) != ""
	})

	// nodetype is a graph entity type name, such as domain or ip.
	_ = v.RegisterValidation("nodetype", func(fl validator.FieldLevel) bool {
		return nodeTypeRe.MatchString(fl.Field().String())
	})

	// keyname is a secret name, such as WHOXY_API_KEY.
	_ = v.RegisterValidation("keyname", func(fl validator.FieldLevel) bool {
		return keyNameRe.MatchString(fl.Field().String())
	})

	// maxbytes bounds a string by its encoded length, which is what
	// bcrypt counts (72), where max counts characters.
	_ = v.RegisterValidation("maxbytes", func(fl validator.FieldLevel) bool {
		n, err := strconv.Atoi(fl.Param())

		return err == nil && len(fl.Field().String()) <= n
	})
}
