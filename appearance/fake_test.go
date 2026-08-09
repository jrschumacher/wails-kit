package appearance

import "sync"

// fakeSource is a controllable Source test double: IsDark reports whatever
// was last set (directly or via flip), and flip both updates it and — like
// a real live Source would — notifies every still-subscribed handler.
// Never touches a real OS API; this is what every test in this package
// (other than the darwin-only parsing tests in os_source_darwin_test.go)
// uses instead of the platform default.
type fakeSource struct {
	mu        sync.Mutex
	dark      bool
	handlers  map[int]func(bool)
	nextID    int
	cancelled int
}

func newFakeSource(dark bool) *fakeSource {
	return &fakeSource{dark: dark, handlers: make(map[int]func(bool))}
}

func (f *fakeSource) IsDark() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dark
}

func (f *fakeSource) Subscribe(fn func(dark bool)) (cancel func()) {
	f.mu.Lock()
	id := f.nextID
	f.nextID++
	f.handlers[id] = fn
	f.mu.Unlock()

	return func() {
		f.mu.Lock()
		delete(f.handlers, id)
		f.cancelled++
		f.mu.Unlock()
	}
}

// flip sets dark and synchronously notifies every currently-subscribed
// handler, simulating a live OS theme change.
func (f *fakeSource) flip(dark bool) {
	f.mu.Lock()
	f.dark = dark
	handlers := make([]func(bool), 0, len(f.handlers))
	for _, fn := range f.handlers {
		handlers = append(handlers, fn)
	}
	f.mu.Unlock()

	for _, fn := range handlers {
		fn(dark)
	}
}

func (f *fakeSource) subscriberCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.handlers)
}
