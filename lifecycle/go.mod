module github.com/jrschumacher/wails-kit/lifecycle

go 1.25.0

require (
	github.com/jrschumacher/wails-kit/errors v0.0.0-00010101000000-000000000000
	github.com/jrschumacher/wails-kit/events v0.0.0-00010101000000-000000000000
)

replace (
	github.com/jrschumacher/wails-kit/errors => ../errors
	github.com/jrschumacher/wails-kit/events => ../events
)
