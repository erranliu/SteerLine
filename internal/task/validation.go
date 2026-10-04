package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/task/model"
)

var controlIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var errCommandMissing = errors.New("task command not found")

func taskError(code, message string) error { return &model.Error{Code: code, Message: message} }

func (c Commands) validateInput() error {
	if c.Input.Authority != "native" {
		return taskError("authority_mismatch", "task_authority=native is required; Beads is not an active product task source")
	}
	return nil
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func lookupTaskCommand(ctx context.Context, store CommandStore, project, id string) (Command, error) {
	command, found, err := store.LookupCommand(ctx, project, id)
	if err == nil && !found {
		err = errCommandMissing
	}
	return command, err
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func validateConfirmationBasis(basis string) (string, error) {
	basis = strings.TrimSpace(basis)
	if basis == "" || !utf8.ValidString(basis) {
		return "", errors.New("recovery confirmation requires a nonempty UTF-8 evidence basis")
	}
	if utf8.RuneCountInString(basis) > 2000 {
		return "", errors.New("recovery confirmation exceeds the 2000 character audit limit; the basis was not truncated")
	}
	return basis, nil
}

func (c Commands) checkTarget(ctx context.Context) error {
	target, err := c.Documents.RepositoryTarget(ctx)
	if err != nil {
		return err
	}
	in := c.Input
	in.Repository = target
	return c.Store.CheckTarget(ctx, in)
}

func (c Commands) readGraph(ctx context.Context) (model.Snapshot, error) {
	target, err := c.Documents.RepositoryTarget(ctx)
	if err != nil {
		return model.Snapshot{}, err
	}
	in := c.Input
	in.Repository = target
	return c.Store.ReadGraph(ctx, in)
}
