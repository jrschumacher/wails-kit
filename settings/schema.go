package settings

import "github.com/jrschumacher/wails-kit/v2/i18n"

type FieldType string

const (
	FieldText     FieldType = "text"
	FieldPassword FieldType = "password"
	FieldSelect   FieldType = "select"
	FieldToggle   FieldType = "toggle"
	FieldComputed FieldType = "computed"
	FieldNumber   FieldType = "number"
)

// SelectOption is one authoring-time option of a select/dynamic-select
// field. Label is i18n.Text (WP-12) — construct it with i18n.T("key", "Other")
// for a translatable option, or i18n.Text{Other: "literal"} (zero Key) for
// one that's never looked up in a catalog. GetSchema resolves Label to a
// plain string on the wire — see ResolvedSelectOption.
type SelectOption struct {
	Label i18n.Text
	Value string
}

// DynamicOptions: select options that vary by another field's value.
// DependsOn is the key of the controlling field.
// Options maps controllingValue -> []SelectOption.
type DynamicOptions struct {
	DependsOn string
	Options   map[string][]SelectOption
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

// Field is the authoring-time representation of one settings field. Label,
// Description, and Placeholder are i18n.Text (WP-12): construct them with
// i18n.T("key", "Other") for a translatable string, or leave the zero
// i18n.Text{} (renders as "") / i18n.Text{Other: "literal"} (renders as
// "literal", never looked up) for one that isn't. GetSchema resolves every
// Text field to a plain string on the wire — see ResolvedField.
//
// Condition and Validation carry no translatable text, so ResolvedField
// reuses these same types directly rather than duplicating them.
type Field struct {
	Key            string
	Type           FieldType
	Label          i18n.Text
	Description    i18n.Text
	Placeholder    i18n.Text
	Default        any
	Options        []SelectOption
	DynamicOptions *DynamicOptions
	Condition      *Condition
	Validation     *Validation
	Advanced       bool
}

type ComputeFunc func(values map[string]any) any

// Group is the authoring-time representation of one settings group. Label
// is i18n.Text — see Field's doc comment.
type Group struct {
	Key          string
	Label        i18n.Text
	Fields       []Field
	ComputeFuncs map[string]ComputeFunc
}

// Schema is the authoring-time representation of a full settings schema —
// what WithGroup composes and Validate operates on. It is never marshaled
// to JSON directly; Service.GetSchema (and Binding.GetSchema) resolve it
// against the service's localizer into a ResolvedSchema, which is the
// actual wire format. See resolve.go.
type Schema struct {
	Groups []Group
}

// ResolvedSelectOption is SelectOption with Label resolved to a plain
// string. This — not SelectOption — is the wire shape; the JSON tags below
// are byte-for-byte what the pre-WP-12 SelectOption used, so no frontend
// change is needed.
type ResolvedSelectOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ResolvedDynamicOptions is DynamicOptions with every option's Label
// resolved.
type ResolvedDynamicOptions struct {
	DependsOn string                            `json:"dependsOn"`
	Options   map[string][]ResolvedSelectOption `json:"options"`
}

// ResolvedField is Field with Label/Description/Placeholder/Options/
// DynamicOptions resolved to plain strings. This is the wire shape GetSchema
// returns; the JSON tags are byte-for-byte what the pre-WP-12 Field used.
type ResolvedField struct {
	Key            string                  `json:"key"`
	Type           FieldType               `json:"type"`
	Label          string                  `json:"label"`
	Description    string                  `json:"description,omitempty"`
	Placeholder    string                  `json:"placeholder,omitempty"`
	Default        any                     `json:"default,omitempty"`
	Options        []ResolvedSelectOption  `json:"options,omitempty"`
	DynamicOptions *ResolvedDynamicOptions `json:"dynamicOptions,omitempty"`
	Condition      *Condition              `json:"condition,omitempty"`
	Validation     *Validation             `json:"validation,omitempty"`
	Advanced       bool                    `json:"advanced,omitempty"`
}

// ResolvedGroup is Group with Label resolved and Fields resolved.
type ResolvedGroup struct {
	Key    string          `json:"key"`
	Label  string          `json:"label"`
	Fields []ResolvedField `json:"fields"`
}

// ResolvedSchema is Schema with every group and field resolved — what
// GetSchema/Binding.GetSchema actually return, and what a frontend renders
// from. JSON shape is unchanged from before WP-12.
type ResolvedSchema struct {
	Groups []ResolvedGroup `json:"groups"`
}
