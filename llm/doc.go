// Package llm is a provider-neutral streaming chat abstraction for desktop
// applications, wired to the settings package so the provider, model and
// credentials are things the user configures rather than things the binary
// hardcodes.
//
// # The Provider abstraction
//
// Everything in this package is expressed in terms of one interface:
//
//	type Provider interface {
//		StreamChat(ctx context.Context, req ChatRequest, handler func(StreamEvent)) error
//		ProviderName() string
//		ModelID() string
//	}
//
// A ChatRequest carries a system prompt, a message history, an optional token
// budget and optional tool definitions. Messages, tool calls, tool results and
// stop reasons are all modelled here, not borrowed from a vendor SDK, so
// application code compiles against this package alone and switching providers
// is a settings change rather than a rewrite.
//
// The protocol constants — Role, EventType, StopReason — are named types whose
// underlying type is string. Untyped literals therefore remain assignable
// (ChatMessage{Role: "user"} still compiles), while the constants give
// discoverable, misspelling-proof names.
//
// # Registration by blank import
//
// This package contains no provider implementations and imports no vendor SDK.
// Implementations live in subpackages that register themselves from init:
//
//	import (
//		"github.com/jrschumacher/wails-kit/llm"
//
//		_ "github.com/jrschumacher/wails-kit/llm/anthropic"
//		_ "github.com/jrschumacher/wails-kit/llm/openai"
//	)
//
// This is deliberate, not incidental. The set of providers a binary can build
// is exactly the set of provider packages it imports, so an application that
// only ships Anthropic support does not link — or vendor, or audit, or ship
// CVE exposure for — the OpenAI SDK, and a third-party provider outside this
// module is a peer of the built-in ones rather than a fork of them. The cost is
// that a missing blank import is a runtime error rather than a compile error;
// NewProvider names the registered providers in its message for that reason,
// and RegisteredProviders reports them directly.
//
// A provider package calls RegisterProvider with a ProviderFactory, plus
// optional RegisterOptions. WithTransportResolver is the one that exists today:
// it lets a provider send its traffic over another registered provider's wire
// protocol, which is how llm/anthropic reaches OpenAI-compatible gateways. That
// knowledge lives in the provider package, so ConfigFromValues stays
// provider-agnostic and a new provider needs no change to this package.
//
// LLMSettingsGroup returns the settings.Group describing provider selection,
// model selection and each provider's connection fields. Its options —
// WithProviders, WithProvider, WithModels, WithExtraModels — cover restricting
// the provider list, adding a provider of your own, and correcting the built-in
// model lists, which are a snapshot and go stale as soon as a vendor ships a
// new model. Called with no options it describes the built-in providers.
//
// llm/mock registers itself the same way, so the whole path from a settings
// values map to a streaming response can be exercised without a network.
//
// # The streaming event contract
//
// StreamChat pushes StreamEvents to handler synchronously, on the calling
// goroutine, and returns when the stream is finished. handler is never called
// after StreamChat returns, so a handler that closes over local state needs no
// locking.
//
// Every stream ends with exactly one terminal event:
//
//   - EventDone on success, carrying the StopReason the model actually stopped
//     for. It is emitted for every terminal condition, not only a natural end
//     of turn — a consumer keyed on "done" is never left hanging because the
//     response hit the token budget or a content filter. StreamChat returns nil.
//   - EventError on failure, carrying the error in Err. The same error is
//     returned from StreamChat, so it can be handled in either place but must
//     not be counted twice. No EventDone follows.
//
// Before the terminal event a stream may deliver any number of EventDelta
// events, each carrying an incremental chunk of assistant text in Text, and at
// most one EventToolUse carrying the fully accumulated tool calls in ToolUses.
// Tool-call arguments arrive fragmented across the wire; EventToolUse is
// emitted only once they are complete, so ToolUseBlock.Input is never a partial
// document. When tool calls are present the StopReason is StopReasonToolUse.
//
// A StopReason the provider reports but this package does not model becomes
// StopReasonUnknown rather than being flattened into StopReasonEndTurn — an
// unrecognized stop must not be mistaken for a completed answer.
//
// # Secrets
//
// API keys are settings password fields, stored in the OS keyring rather than
// in the settings file. settings.Service exposes them two ways, and the
// difference matters:
//
//   - GetValues masks every password field with settings.SecretSentinel. It is
//     what crosses the Wails bridge to the frontend.
//   - GetValuesWithSecrets resolves the real values. It must never leave the Go
//     side.
//
// ProviderManager is the safe path: it calls GetValuesWithSecrets internally
// and the resolved map never leaves that call, so application code holds a
// working Provider without ever holding the key.
//
//	mgr := llm.NewProviderManager(svc)
//	p, err := mgr.Provider()   // lazily built, then cached
//	err = mgr.Reload()         // after the user changes settings
//
// NewProviderFromValues takes a raw values map and is the trap: passing it
// GetValues output compiles cleanly and yields a provider that authenticates
// with the placeholder, failing as an opaque 401 from someone else's API far
// from the mistake. It therefore rejects a map whose API key is
// settings.SecretSentinel. Callers that build a values map by hand must supply
// the real key.
package llm
