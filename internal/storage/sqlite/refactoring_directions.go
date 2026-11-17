package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/refactoring/model"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// RefactoringDirections owns the direction catalogue shared by one installation.
type RefactoringDirections struct{ Home string }

func (s RefactoringDirections) open(ctx context.Context) (*sql.DB, error) {
	if err := os.MkdirAll(s.Home, 0700); err != nil {
		return nil, err
	}
	path := filepath.ToSlash(filepath.Join(s.Home, "refactoring-directions.db"))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	uri := url.URL{Scheme: "file", Path: path, RawQuery: "_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS refactoring_directions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		name_key TEXT NOT NULL UNIQUE,
		prompt TEXT NOT NULL,
		revision TEXT NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s RefactoringDirections) List(ctx context.Context) ([]model.Direction, error) {
	db, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT id, name, prompt, revision FROM refactoring_directions ORDER BY name_key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var directions []model.Direction
	for rows.Next() {
		var d model.Direction
		if err := rows.Scan(&d.ID, &d.Name, &d.Prompt, &d.Revision); err != nil {
			return nil, err
		}
		directions = append(directions, d)
	}
	return directions, rows.Err()
}

func (s RefactoringDirections) Read(ctx context.Context, id string) (model.Direction, error) {
	var d model.Direction
	db, err := s.open(ctx)
	if err != nil {
		return d, err
	}
	defer db.Close()
	err = db.QueryRowContext(ctx, "SELECT id, name, prompt, revision FROM refactoring_directions WHERE id = ?", id).
		Scan(&d.ID, &d.Name, &d.Prompt, &d.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		err = model.ErrDirectionMissing
	}
	return d, err
}

func (s RefactoringDirections) Change(ctx context.Context, in model.DirectionChange) (model.Direction, error) {
	var d model.Direction
	switch in.Action {
	case "create":
		if in.ID != "" || in.Revision != "" {
			return d, model.ErrDirectionInvalid
		}
	case "update", "delete":
		if !model.ValidID(in.ID) || !model.ValidDirectionRevision(in.Revision) {
			return d, model.ErrDirectionInvalid
		}
	default:
		return d, model.ErrDirectionInvalid
	}
	name := strings.TrimSpace(in.Name)
	key := strings.ToLower(name)
	if in.Action == "delete" {
		if in.Name != "" || in.Prompt != "" {
			return d, model.ErrDirectionInvalid
		}
	} else {
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 ||
			strings.ContainsFunc(name, unicode.IsControl) {
			return d, model.DirectionInputError("方向名称必填，最多 100 个字符，且不能含控制字符")
		}
		if key == model.DefaultDirectionName {
			return d, model.ErrDirectionDuplicate
		}
		if err := model.ValidateDirectionPrompt(in.Prompt); err != nil {
			return d, err
		}
		d = model.Direction{ID: in.ID, Name: name, Prompt: in.Prompt, Revision: model.Digest([]byte(model.ID()))}
	}
	db, err := s.open(ctx)
	if err != nil {
		return model.Direction{}, err
	}
	defer db.Close()
	var result sql.Result
	switch in.Action {
	case "create":
		d.ID = model.ID()
		result, err = db.ExecContext(ctx, "INSERT INTO refactoring_directions (id, name, name_key, prompt, revision) VALUES (?, ?, ?, ?, ?)", d.ID, name, key, d.Prompt, d.Revision)
	case "update":
		result, err = db.ExecContext(ctx, "UPDATE refactoring_directions SET name = ?, name_key = ?, prompt = ?, revision = ? WHERE id = ? AND revision = ?", name, key, d.Prompt, d.Revision, d.ID, in.Revision)
	case "delete":
		result, err = db.ExecContext(ctx, "DELETE FROM refactoring_directions WHERE id = ? AND revision = ?", in.ID, in.Revision)
	}
	if err != nil {
		var failure *driver.Error
		if errors.As(err, &failure) && failure.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return model.Direction{}, model.ErrDirectionDuplicate
		}
		return model.Direction{}, err
	}
	if count, err := result.RowsAffected(); err != nil {
		return model.Direction{}, err
	} else if count != 1 {
		return model.Direction{}, model.ErrDirectionConflict
	}
	return d, nil
}
