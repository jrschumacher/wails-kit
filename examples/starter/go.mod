// The starter example is its own Go module, and that is the point.
//
// It depends on Wails v3; the wails-kit root module does not, and must not.
// Keeping the example in a separate module is what stops `go get
// github.com/jrschumacher/wails-kit` from dragging a beta desktop framework
// into every consumer's dependency graph.
module github.com/jrschumacher/wails-kit/examples/starter

go 1.25.0

require (
	github.com/jrschumacher/wails-kit v0.0.0
	github.com/wailsapp/wails/v3 v3.0.0-beta.4
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/anthropics/anthropic-sdk-go v1.19.0 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/openai/openai-go/v2 v2.7.1 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

// Build against the checkout this example lives in rather than a published
// tag, so the example always exercises the current source.
replace github.com/jrschumacher/wails-kit => ../..
