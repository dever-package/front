package validate

import (
	"github.com/shemic/dever/util"

	frontpage "my/package/front/service/page"
)

type validateEnvelope struct {
	Nodes map[string][]validateItem `json:"nodes"`
}

type validateItem struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Value    string         `json:"value"`
	Mode     string         `json:"mode"`
	Meta     map[string]any `json:"meta"`
	Validate []validateRule `json:"validate"`
}

type validateRule struct {
	Type      string              `json:"type"`
	Message   string              `json:"message"`
	Pattern   string              `json:"pattern"`
	Target    string              `json:"target"`
	Min       *float64            `json:"min"`
	Max       *float64            `json:"max"`
	Model     string              `json:"model"`
	Service   string              `json:"service"`
	Field     string              `json:"field"`
	Operator  string              `json:"operator"`
	Except    string              `json:"except"`
	Params    any                 `json:"params"`
	When      []validateCondition `json:"when"`
	Condition string              `json:"condition"`
}

type validateCondition struct {
	Path     string `json:"path"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type Failure struct {
	Field   string
	Message string
}

var validateItemsCache util.ConcurrentMap[frontpage.ContentSignature, []validateItem]
