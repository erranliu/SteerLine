package app

import (
	"context"
	"encoding/json"
	"time"
)

// startupQueries belongs to one project's preparation phase, never to a live
// actor or a later command. Acquiring ownership starts a new phase. Filesystem,
// SQLite and lock checks still run at their original admission boundaries.
type startupQueries struct {
	closed                            bool
	ctx                               context.Context
	identityKey, repository, identity string
	git                               map[string][]byte
}

func newStartupQueries(ctx context.Context) *startupQueries {
	return &startupQueries{ctx: ctx, git: make(map[string][]byte)}
}

func (c *Config) finishStartupQueries() {
	if c.startup != nil {
		c.startup.closed = true
		c.startup.git = nil
		c.startup = nil
	}
}

func (c *Config) resetStartupQueries(ctx context.Context) {
	c.finishStartupQueries()
	c.startup = newStartupQueries(ctx)
}

func (c Config) queryContext(ctx context.Context) context.Context {
	if c.startup != nil && !c.startup.closed && c.startup.ctx != nil && ctx.Done() == nil {
		return c.startup.ctx
	}
	return ctx
}

func (c Config) identityQueryKey() string {
	data, _ := json.Marshal([]any{c.Repository, c.Git.Executable, c.Git.Args})
	return string(data)
}

func (c Config) startupGitOutput(ctx context.Context, args ...string) ([]byte, error) {
	ctx = c.queryContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keyBytes, _ := json.Marshal([]any{c.identityQueryKey(), args})
	key := string(keyBytes)
	if c.startup != nil && !c.startup.closed {
		if value, ok := c.startup.git[key]; ok {
			return append([]byte(nil), value...), nil
		}
	}
	runner := commandRunner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), ProcessHost: c.ProcessHost}
	value, err := runner.Output(ctx, c.Git, args...)
	if err == nil && c.startup != nil && !c.startup.closed {
		c.startup.git[key] = append([]byte(nil), value...)
	}
	return value, err
}
