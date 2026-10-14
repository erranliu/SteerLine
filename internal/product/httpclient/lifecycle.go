package httpclient

// NewPlatformStopObserver returns the native process/lock observer for the
// current operating system. Composition roots inject it explicitly.
func NewPlatformStopObserver() StopObserver { return observeExit }
