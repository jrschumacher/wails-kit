package settings

import "context"

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
	Check          *Check          `json:"check,omitempty"`
	Advanced       bool            `json:"advanced,omitempty"`
}

// Check marks a field whose value the back end checks before it applies (a
// token, a path). The check itself is the group's CheckFuncs entry for the
// field; Service.CheckValue runs it. Pending is the words shown beside the
// field while the check runs, naming the work ("Checking token…"); the
// settings window says "Checking…" without them.
type Check struct {
	Pending string `json:"pending,omitempty"`
}

type ComputeFunc func(values map[string]any) any

// CheckFunc checks a field's value with the back end (a token against its
// service, a path on disk). It returns nil when the value may apply. A
// failure's message for the person is its *errors.UserError's user message
// (errors.New with a code registered through errors.RegisterMessages); any
// other error shows ErrCheckFailed's message. ctx ends at the check timeout
// (WithCheckTimeout); it does not end when the window that asked closes.
type CheckFunc func(ctx context.Context, value any, values map[string]any) error

type Group struct {
	Key          string                 `json:"key"`
	Label        string                 `json:"label"`
	Fields       []Field                `json:"fields"`
	ComputeFuncs map[string]ComputeFunc `json:"-"`
	CheckFuncs   map[string]CheckFunc   `json:"-"`
}

type Schema struct {
	Groups []Group `json:"groups"`
}
