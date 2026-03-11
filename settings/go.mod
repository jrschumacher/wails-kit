module github.com/jrschumacher/wails-kit/settings

go 1.25.0

require (
	github.com/jrschumacher/wails-kit/appdirs v0.0.0-00010101000000-000000000000
	github.com/jrschumacher/wails-kit/keyring v0.0.0-00010101000000-000000000000
)

require (
	al.essio.dev/pkg/shellescape v1.5.1 // indirect
	github.com/danieljoos/wincred v1.2.2 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/zalando/go-keyring v0.2.6 // indirect
	golang.org/x/sys v0.26.0 // indirect
)

replace (
	github.com/jrschumacher/wails-kit/appdirs => ../appdirs
	github.com/jrschumacher/wails-kit/keyring => ../keyring
)
