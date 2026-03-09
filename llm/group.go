package llm

import (
	"github.com/jrschumacher/wails-kit/settings"
)

func LLMSettingsGroup() settings.Group {
	return settings.Group{
		Key:   "llm",
		Label: "LLM",
		Fields: []settings.Field{
			{
				Key:     "llm.provider",
				Type:    settings.FieldSelect,
				Label:   "Provider",
				Default: "anthropic",
				Options: []settings.SelectOption{
					{Label: "Anthropic", Value: "anthropic"},
					{Label: "OpenAI", Value: "openai"},
				},
			},
			{
				Key:     "llm.model",
				Type:    settings.FieldSelect,
				Label:   "Model",
				Default: "claude-sonnet-4-6",
				DynamicOptions: &settings.DynamicOptions{
					DependsOn: "llm.provider",
					Options: map[string][]settings.SelectOption{
						"anthropic": {
							{Label: "Claude Sonnet 4.6", Value: "claude-sonnet-4-6"},
							{Label: "Claude Opus 4.6", Value: "claude-opus-4-6"},
							{Label: "Claude Haiku 4.5", Value: "claude-haiku-4-5-20251001"},
						},
						"openai": {
							{Label: "GPT-4o", Value: "gpt-4o"},
							{Label: "GPT-4o Mini", Value: "gpt-4o-mini"},
							{Label: "o3", Value: "o3"},
						},
					},
				},
			},
			{
				Key:      "llm.anthropic.baseURL",
				Type:     settings.FieldText,
				Label:    "Base URL",
				Advanced: true,
				Condition: &settings.Condition{
					Field:  "llm.provider",
					Equals: []string{"anthropic"},
				},
			},
			{
				Key:      "llm.anthropic.secret",
				Type:     settings.FieldPassword,
				Label:    "API Key",
				Advanced: true,
				Condition: &settings.Condition{
					Field:  "llm.provider",
					Equals: []string{"anthropic"},
				},
			},
			{
				Key:      "llm.anthropic.apiFormat",
				Type:     settings.FieldSelect,
				Label:    "API Format",
				Default:  "anthropic-native",
				Advanced: true,
				Condition: &settings.Condition{
					Field:  "llm.provider",
					Equals: []string{"anthropic"},
				},
				Options: []settings.SelectOption{
					{Label: "Anthropic Native", Value: "anthropic-native"},
					{Label: "OpenAI Compatible", Value: "openai-compatible"},
				},
			},
			{
				Key:      "llm.anthropic.customModel",
				Type:     settings.FieldText,
				Label:    "Custom Model ID",
				Advanced: true,
				Condition: &settings.Condition{
					Field:  "llm.provider",
					Equals: []string{"anthropic"},
				},
			},
			{
				Key:      "llm.openai.baseURL",
				Type:     settings.FieldText,
				Label:    "Base URL",
				Advanced: true,
				Condition: &settings.Condition{
					Field:  "llm.provider",
					Equals: []string{"openai"},
				},
			},
			{
				Key:      "llm.openai.secret",
				Type:     settings.FieldPassword,
				Label:    "API Key",
				Advanced: true,
				Condition: &settings.Condition{
					Field:  "llm.provider",
					Equals: []string{"openai"},
				},
			},
			{
				Key:      "llm.openai.customModel",
				Type:     settings.FieldText,
				Label:    "Custom Model ID",
				Advanced: true,
				Condition: &settings.Condition{
					Field:  "llm.provider",
					Equals: []string{"openai"},
				},
			},
			{
				Key:      "llm.resolvedModelID",
				Type:     settings.FieldComputed,
				Label:    "Resolved Model ID",
				Advanced: true,
			},
		},
		ComputeFuncs: map[string]settings.ComputeFunc{
			"llm.resolvedModelID": computeResolvedModelID,
		},
	}
}

func computeResolvedModelID(values map[string]any) any {
	return ResolveModelID(values)
}
