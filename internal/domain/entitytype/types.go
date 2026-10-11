// Gosint, Copyright (c) 2026 YouSysAdmin

package entitytype

// detectInput is values to recognize, one each.
type detectInput struct {
	Values []string `json:"values" validate:"required,min=1,max=10000,dive,max=4096"`
}

// fieldView is one field of a type as the console renders it.
type fieldView struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"`
	Elem        string   `json:"elem"`
	Ref         string   `json:"ref"`
	Required    bool     `json:"required"`
	Primary     bool     `json:"primary"`
	Long        bool     `json:"long"`
	Default     any      `json:"default"`
	Options     []string `json:"options"`
	Min         *float64 `json:"min"`
	Max         *float64 `json:"max"`
	VaultKey    bool     `json:"vault_key"`
	Secret      bool     `json:"secret"`
}

// typeView is one entity type.
type typeView struct {
	Name        string      `json:"name"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Category    string      `json:"category"`
	Icon        string      `json:"icon"`
	Color       string      `json:"color"`
	Detectable  bool        `json:"detectable"`
	Fields      []fieldView `json:"fields"`
}

// categoryView is one palette group.
type categoryView struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Icon  string `json:"icon"`
}

// typesResponse answers the catalog.
type typesResponse struct {
	Categories []categoryView `json:"categories"`
	Types      []typeView     `json:"types"`
}

// detection is what one value was recognized as. Type is phrase when
// nothing matched, and Error says why a match did not parse.
type detection struct {
	Value      string         `json:"value"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Error      string         `json:"error"`
}

// detectResponse answers a detection, in input order.
type detectResponse struct {
	Results []detection `json:"results"`
}
