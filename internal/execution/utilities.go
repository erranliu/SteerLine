package execution

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
func shortHash(s string) string          { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:12]) }
func operationDigest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func canonical(s string) string {
	s = filepath.Clean(s)
	if runtime.GOOS == "windows" {
		return strings.ToLower(s)
	}
	return s
}
func nativeErr(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }
func nativeTaskRecord(tasks []taskmodel.TaskRecord, id string) *taskmodel.TaskRecord {
	for i := range tasks {
		if tasks[i].TaskID == id {
			return &tasks[i]
		}
	}
	return nil
}
func requireNativeTaskAuthority(c Config) error {
	if c.TaskAuthority != "native" {
		return errors.New("task_authority=native is required; Beads is not an active product task source")
	}
	return nil
}

func recordFingerprint(value any) string { return operationDigest(mustJSON(value)) }

type commandReplayed struct{ Status string }

func (commandReplayed) Error() string { return "command already applied" }

func defaultRequestedBy() string {
	for _, key := range []string{"STEERLINE_ANSWERED_BY", "USERNAME", "USER"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "unknown"
}

func resolveProspectivePath(input string) (string, error) {
	path, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return canonical(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}

func ptr[T any](value T) *T { return &value }
func now() time.Time        { return time.Now().UTC() }

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

const ActivityVersion = 1
