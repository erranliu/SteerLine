package persistence

// StorageError preserves an implementation failure while exposing a stable
// classification to consumers without requiring a driver or SQL dependency.
type StorageError struct {
	Code  string
	Cause error
}

func (e *StorageError) Error() string { return e.Code + ": " + e.Cause.Error() }
func (e *StorageError) Unwrap() error { return e.Cause }

const (
	StorageLocked       = "storage_locked"
	StorageIO           = "storage_io"
	StorageCommitFailed = "storage_commit_failed"
)
