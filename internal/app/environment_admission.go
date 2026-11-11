package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/erranliu/SteerLine/internal/executionconfig"
)

// One home-owned mutex covers only new target work reservations. Durable
// project records, rather than an in-memory busy flag, own the occupancy.
type environmentAdmission struct {
	mu       sync.Mutex
	projects map[string]Config
}

func (a *environmentAdmission) register(c Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.projects == nil {
		a.projects = map[string]Config{}
	}
	a.projects[c.Identity()] = c
}
func (a *environmentAdmission) check(ctx context.Context, target, owner string) error {
	for _, c := range a.projects {
		if _, e := os.Stat(filepath.Join(c.RunDir, "steerline.db")); os.IsNotExist(e) {
			continue
		} else if e != nil {
			return e
		}
		reader, e := OpenNativeTaskReader(ctx, &c)
		if e != nil {
			return e
		}
		rows, e := reader.Environments(ctx, c.Identity())
		if e != nil {
			reader.Close()
			return e
		}
		for _, r := range rows {
			if r.Kind == "adaptation" && r.Project+"\n"+r.ID != owner && executionconfig.Target(r.Execution) == target && !r.Settled() {
				reader.Close()
				return errors.New("环境未就绪：该目标环境仍有未核实的环境适配")
			}
		}
		if e := reader.Close(); e != nil {
			return e
		}
	}
	return nil
}
func (a *environmentAdmission) adapt(ctx context.Context, target, owner string, save func() error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if e := a.check(ctx, target, owner); e != nil {
		return e
	}
	return save()
}
