// Gosint, Copyright (c) 2026 YouSysAdmin

package transform

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
)

// SecretFunc opens a secret by name or id. False when there is none.
type SecretFunc func(ref string) (string, bool, error)

// ParamError is a parameter the caller has to fix.
type ParamError struct {
	Name    string
	Message string
}

// Error renders the refusal.
func (e *ParamError) Error() string { return e.Name + " " + e.Message }

// ResolveParams checks given against the transform's params and returns
// every value to run with: given values, defaults, and secrets opened
// from the stored secrets.
//
// Unknown names are refused, so a typo in a parameter is an error
// rather than a silently ignored setting.
func ResolveParams(params []Param, given map[string]string, secret SecretFunc) (map[string]string, error) {
	known := map[string]bool{}
	for _, p := range params {
		known[p.Name] = true
	}

	for name := range given {
		if !known[name] {
			return nil, &ParamError{Name: name, Message: "is not a parameter of this transform"}
		}
	}

	out := map[string]string{}
	for _, p := range params {
		v := given[p.Name]
		if p.Type == ParamSecret {
			opened, err := openSecret(p, v, secret)
			if err != nil {
				return nil, err
			}

			if opened != "" {
				out[p.Name] = opened
			}

			continue
		}

		if v == "" {
			v = p.Default
		}

		if v == "" {
			if p.Required {
				return nil, &ParamError{Name: p.Name, Message: "is required"}
			}

			continue
		}

		if err := checkValue(p, v); err != nil {
			return nil, err
		}

		out[p.Name] = v
	}

	return out, nil
}

// openSecret resolves a secret parameter: the secret the caller
// named, or else the key named like the parameter.
func openSecret(p Param, ref string, secret SecretFunc) (string, error) {
	for _, name := range []string{ref, p.Name} {
		if name == "" {
			continue
		}

		v, ok, err := secret(name)
		if err != nil {
			return "", fmt.Errorf("open secret for %s: %w", p.Name, err)
		}

		if ok {
			return v, nil
		}

		if name == ref {
			return "", &ParamError{Name: p.Name, Message: fmt.Sprintf("names secret %q, which does not exist", ref)}
		}
	}

	if p.Default != "" {
		return p.Default, nil
	}

	if p.Required {
		return "", &ParamError{Name: p.Name, Message: fmt.Sprintf("needs the secret %s, add it in Secrets", p.Name)}
	}

	return "", nil
}

// numberRe is a plain decimal number.
var numberRe = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

func checkValue(p Param, v string) error {
	switch p.Type {
	case ParamNumber:
		// Plain decimal only: ParseFloat also takes NaN, Inf, 1e3 and hex,
		// which a caller reading an int turns into zero.
		if !numberRe.MatchString(v) {
			return &ParamError{Name: p.Name, Message: "must be a number"}
		}
	case ParamURL:
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return &ParamError{Name: p.Name, Message: "must be an http or https URL"}
		}
	case ParamSelect:
		if !slices.ContainsFunc(p.Options, func(o Option) bool { return o.Value == v }) {
			return &ParamError{Name: p.Name, Message: "must be one of the listed options"}
		}
	case ParamBool:
		if v != "true" && v != "false" {
			return &ParamError{Name: p.Name, Message: "must be true or false"}
		}
	case ParamString:
	default:
		return errors.New("transform: parameter " + p.Name + " has unknown type " + string(p.Type))
	}

	return nil
}
