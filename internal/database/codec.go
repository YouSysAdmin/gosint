// Gosint, Copyright (c) 2026 YouSysAdmin

package database

// Codec seals column values at rest. The vault implements it, a nil
// vault stores clear text, which only tests use.
type Codec interface {
	SealText(aad, plain string) (string, error)
	OpenText(aad, stored string) (string, error)

	// Index is a keyed hash for matching a sealed value by equality.
	Index(purpose, value string) (string, error)
}

// AAD names where a sealed value is stored, so it opens nowhere else.
func AAD(table, column, id string) string {
	return table + "." + column + ":" + id
}

// SealJSON seals v as JSON for a TEXT column.
func SealJSON(c Codec, aad string, v any) (string, error) {
	return c.SealText(aad, MustJSON(v))
}

// OpenJSON reads what SealJSON wrote.
func OpenJSON(c Codec, aad, stored string, dst any) error {
	plain, err := c.OpenText(aad, stored)
	if err != nil {
		return err
	}

	return UnmarshalJSON(plain, dst)
}
