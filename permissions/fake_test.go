package permissions

import "sync"

// fakePlatform is the platform double every test in this package builds
// on — it never touches cgo, a real display, or an OS permission prompt,
// so the whole state machine (Check/Request/OpenSystemSettings, the
// denied/not_determined synthesis, context handling) is exercised
// headlessly. Configure per-Kind behavior with the granted/supported maps
// before constructing a Service via newService; requestBlock lets a test
// hold Request open until it explicitly unblocks it, for context
// cancellation coverage.
type fakePlatform struct {
	mu sync.Mutex

	granted    map[Kind]bool
	supported  map[Kind]bool // absent (zero value false) means unsupported
	requestErr map[Kind]error

	// requestBlock, if non-nil, makes request(k) wait on this channel
	// before returning — used to test Request racing ctx cancellation.
	requestBlock chan struct{}

	openSettingsErr   error
	openSettingsCalls []Kind
	requestCalls      []Kind
	checkCalls        []Kind
}

func newFakePlatform() *fakePlatform {
	return &fakePlatform{
		granted:    make(map[Kind]bool),
		supported:  make(map[Kind]bool),
		requestErr: make(map[Kind]error),
	}
}

func (f *fakePlatform) setSupported(k Kind, granted bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.supported[k] = true
	f.granted[k] = granted
}

func (f *fakePlatform) check(k Kind) (granted bool, supported bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCalls = append(f.checkCalls, k)
	return f.granted[k], f.supported[k]
}

func (f *fakePlatform) request(k Kind) (granted bool, supported bool, err error) {
	f.mu.Lock()
	f.requestCalls = append(f.requestCalls, k)
	block := f.requestBlock
	f.mu.Unlock()

	if block != nil {
		<-block
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	return f.granted[k], f.supported[k], f.requestErr[k]
}

func (f *fakePlatform) openSystemSettings(k Kind) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openSettingsCalls = append(f.openSettingsCalls, k)
	return f.openSettingsErr
}
