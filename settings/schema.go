package settings

type FieldType string

const (
	FieldText     FieldType = "text"
	FieldPassword FieldType = "password"
	FieldSelect   FieldType = "select"
	FieldToggle   FieldType = "toggle"
	FieldComputed FieldType = "computed"
	FieldNumber   FieldType = "number"
)

type SelectOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// DynamicOptions: select options that vary by another field's value.
// DependsOn is the key of the controlling field.
// Options maps controllingValue -> []SelectOption.
type DynamicOptions struct {
	DependsOn string                    `json:"dependsOn"`
	Options   map[string][]SelectOption `json:"options"`
}

// Condition: show this field only when the controlling field's value is in Equals.
type Condition struct {
	Field  string   `json:"field"`
	Equals []string `json:"equals"`
}

type Validation struct {
	Required bool   `json:"required,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	MinLen   int    `json:"minLen,omitempty"`
	MaxLen   int    `json:"maxLen,omitempty"`
	Min      *int   `json:"min,omitempty"`
	Max      *int   `json:"max,omitempty"`
}

type Field struct {
	Key            string          `json:"key"`
	Type           FieldType       `json:"type"`
	Label          string          `json:"label"`
	Description    string          `json:"description,omitempty"`
	Placeholder    string          `json:"placeholder,omitempty"`
	Default        any             `json:"default,omitempty"`
	Options        []SelectOption  `json:"options,omitempty"`
	DynamicOptions *DynamicOptions `json:"dynamicOptions,omitempty"`
	Condition      *Condition      `json:"condition,omitempty"`
	Validation     *Validation     `json:"validation,omitempty"`
	Advanced       bool            `json:"advanced,omitempty"`
}

type ComputeFunc func(values map[string]any) any

type Group struct {
	Key          string                 `json:"key"`
	Label        string                 `json:"label"`
	Fields       []Field                `json:"fields"`
	ComputeFuncs map[string]ComputeFunc `json:"-"`
}

type Schema struct {
	Groups []Group `json:"groups"`
}
