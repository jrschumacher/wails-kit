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

// Condition shows this field only when the controlling field's value equals one
// of the values in Equals.
//
// Equals is []any so that a field can be gated on a toggle or a number, not
// only on a select. It serializes as a plain JSON array of literals
// (`{"field":"proxy.enabled","equals":[true]}`), which is what a frontend
// renderer evaluating conditions itself should expect.
//
// Comparison semantics — see conditionMet for the implementation:
//
//   - Numbers compare numerically across every Go numeric type. A schema
//     written as Equals: []any{3} matches a value of 3, int64(3) or float64(3),
//     so it behaves identically whether the value came from Go or across the
//     JSON bridge (where all numbers arrive as float64).
//   - Bools compare to bools, strings to strings.
//   - nil matches only nil. A field key that is absent from the value map is
//     nil, so Equals: []any{nil} means "while this field is unset".
//   - Comparison never crosses kinds: "true" does not match true, and "1" does
//     not match 1. Coercing across kinds would make a schema's behaviour depend
//     on which side of the bridge the value happened to come from.
//   - An empty or nil Equals never matches, so the field is always hidden.
type Condition struct {
	Field  string `json:"field"`
	Equals []any  `json:"equals"`
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
