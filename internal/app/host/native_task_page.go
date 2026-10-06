package host

type HostError struct {
	Code    string
	Message string
}

func (e *HostError) Error() string { return e.Code + ": " + e.Message }

func nativeErr(code, message string) error { return &HostError{Code: code, Message: message} }
