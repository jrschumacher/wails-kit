module github.com/jrschumacher/wails-kit/llm/openai

go 1.25.0

require (
	github.com/jrschumacher/wails-kit/llm v0.0.0-00010101000000-000000000000
	github.com/jrschumacher/wails-kit/logging v0.0.0-00010101000000-000000000000
	github.com/openai/openai-go/v2 v2.7.1
)

require (
	al.essio.dev/pkg/shellescape v1.5.1 // indirect
	github.com/danieljoos/wincred v1.2.2 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jrschumacher/wails-kit/appdirs v0.0.0-00010101000000-000000000000 // indirect
	github.com/jrschumacher/wails-kit/keyring v0.0.0-00010101000000-000000000000 // indirect
	github.com/jrschumacher/wails-kit/settings v0.0.0-00010101000000-000000000000 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/zalando/go-keyring v0.2.6 // indirect
	golang.org/x/sys v0.29.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

replace (
	github.com/jrschumacher/wails-kit/appdirs => ../../appdirs
	github.com/jrschumacher/wails-kit/keyring => ../../keyring
	github.com/jrschumacher/wails-kit/llm => ../
	github.com/jrschumacher/wails-kit/logging => ../../logging
	github.com/jrschumacher/wails-kit/settings => ../../settings
)
