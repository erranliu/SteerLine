package sqlite

import (
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
)

func openAsyncActivityPool(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(100)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	// Do not retain WAL/SHM handles when the activity view is idle on Windows.
	db.SetMaxIdleConns(0)
	return db, nil
}

func (s *SQLiteStore) asyncActivityPool() (*sql.DB, error) {
	if s.readOnly {
		return s.db, nil
	}
	if s.activityErr != nil {
		return nil, s.activityErr
	}
	if s.activityDB == nil {
		return nil, errors.New("activity read-only pool unavailable")
	}
	return s.activityDB, nil
}
