module github.com/jrschumacher/wails-kit/settings/templates/anyllm/v2

go 1.25.0

// Local development resolves the main module via the repo-root go.work, not
// this replace — the require+replace pair below exists so `go build`/`go
// test` inside this directory also work standalone (no GOWORK), and so
// `go mod tidy` has a real version to pin once wails-kit/v2 is tagged. See
// README.md "Release tagging" for what to do at release time: drop the
// replace and bump the require to the real tag.
require github.com/jrschumacher/wails-kit/v2 v2.0.0-00010101000000-000000000000

require github.com/mozilla-ai/any-llm-go v0.9.0

require (
	al.essio.dev/pkg/shellescape v1.6.0 // indirect
	cloud.google.com/go v0.116.0 // indirect
	cloud.google.com/go/auth v0.9.3 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/anthropics/anthropic-sdk-go v1.26.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/ollama/ollama v0.17.6 // indirect
	github.com/openai/openai-go v1.12.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/zalando/go-keyring v0.2.6 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genai v1.49.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260217215200-42d3e9bedb6d // indirect
	google.golang.org/grpc v1.79.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/jrschumacher/wails-kit/v2 => ../../..
