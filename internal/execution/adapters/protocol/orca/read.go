package orca

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

var ErrStreamGap = errors.New("original terminal output is clipped, missing or reset")

type StreamPage struct {
	Handle       string   `json:"handle"`
	Source       string   `json:"source"`
	Tail         []string `json:"tail"`
	OldestCursor string   `json:"oldestCursor"`
	NextCursor   string   `json:"nextCursor"`
	LatestCursor string   `json:"latestCursor"`
	Limited      bool     `json:"limited"`
	Truncated    bool     `json:"truncated"`
}

func ValidCursor(value string) bool {
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil && value != ""
}

func (c Client) stream(ctx context.Context, p b.ProcessIdentity, cursor string, limit int) (StreamPage, error) {
	args := []string{"terminal", "read", "--terminal", p.Terminal, "--limit", strconv.Itoa(limit), "--json"}
	if cursor != "" {
		args = append(args, "--cursor", cursor)
	}
	r, err := c.MetaCall(ctx, args...)
	if err != nil {
		return StreamPage{}, err
	}
	var value struct {
		Terminal StreamPage `json:"terminal"`
	}
	if json.Unmarshal(r.Payload, &value) != nil || r.RuntimeID != p.RuntimeID || value.Terminal.Handle != p.Terminal || value.Terminal.Source != "stream" && value.Terminal.Source != "screen" || !ValidCursor(value.Terminal.NextCursor) || !ValidCursor(value.Terminal.LatestCursor) || !ValidCursor(value.Terminal.OldestCursor) {
		return StreamPage{}, errors.New("original Orca terminal stream identity/cursor is unverified")
	}
	for _, line := range value.Terminal.Tail {
		if !utf8.ValidString(line) {
			return StreamPage{}, errors.New("invalid terminal output")
		}
	}
	return value.Terminal, nil
}

// CaptureOutput fixes the public CLI output source for this original turn.
func (c Client) CaptureOutput(ctx context.Context, p b.ProcessIdentity) (StreamPage, error) {
	return c.stream(ctx, p, "", 256)
}
func (c Client) CaptureCursor(ctx context.Context, p b.ProcessIdentity) (string, error) {
	page, err := c.stream(ctx, p, "0", 1)
	return page.LatestCursor, err
}
func (c Client) ReadScreen(ctx context.Context, p b.ProcessIdentity) (StreamPage, error) {
	page, err := c.stream(ctx, p, "", 256)
	if err != nil {
		return page, err
	}
	if page.Source != "screen" {
		return page, errors.New("original output source changed")
	}
	if page.Truncated || page.Limited {
		return page, ErrStreamGap
	}
	return page, nil
}

// ReadAfter never falls back to a screen snapshot or another output source.
func (c Client) ReadAfter(ctx context.Context, p b.ProcessIdentity, cursor string) (StreamPage, error) {
	if !ValidCursor(cursor) {
		return StreamPage{}, errors.New("missing original output cursor")
	}
	page, err := c.stream(ctx, p, cursor, 256)
	if err != nil {
		return page, err
	}
	previous, _ := strconv.ParseUint(cursor, 10, 64)
	next, _ := strconv.ParseUint(page.NextCursor, 10, 64)
	latest, _ := strconv.ParseUint(page.LatestCursor, 10, 64)
	oldest, _ := strconv.ParseUint(page.OldestCursor, 10, 64)
	if page.Source != "stream" {
		return page, errors.New("original output source changed")
	}
	if page.Truncated || oldest > previous || next < previous || latest < next || !page.Limited && next != latest || page.Limited && next == previous && next != latest {
		return page, ErrStreamGap
	}
	return page, nil
}

// Logical terminal lines may soft-wrap the requested single-line JSON frame.
func (p StreamPage) Text() string { return strings.Join(p.Tail, "") }
