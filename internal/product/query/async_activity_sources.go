package query

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

// Includes connection wait, SQL and decoding, independently for each source.
const AsyncActivitySourceBudget = time.Second

type activitySourceSample struct {
	source      api.AsyncActivitySource
	rows        []ports.AsyncActivityFact
	more        bool
	projected   map[string]api.AsyncActivity
	counts      map[string]int
	countsKnown bool
	links       map[string]api.AsyncActivity
}

func sampleActivitySources(ctx context.Context, reader ports.AsyncActivityReader, project, instance string, sources []string, o AsyncActivityOptions, now time.Time, linksOnly bool) []activitySourceSample {
	samples := make([]activitySourceSample, len(sources))
	var group sync.WaitGroup
	for index, source := range sources {
		group.Add(1)
		go func() {
			defer group.Done()
			samples[index] = sampleActivitySource(ctx, reader, project, instance, source, o, now, linksOnly)
		}()
	}
	group.Wait()
	return samples
}

func sampleActivitySource(parent context.Context, reader ports.AsyncActivityReader, project, instance, source string, o AsyncActivityOptions, now time.Time, linksOnly bool) activitySourceSample {
	ctx, cancel := context.WithTimeout(parent, AsyncActivitySourceBudget)
	defer cancel()
	s := activitySourceSample{source: api.AsyncActivitySource{Source: source, Complete: true}, projected: map[string]api.AsyncActivity{}, counts: map[string]int{}, countsKnown: !linksOnly, links: map[string]api.AsyncActivity{}}
	fail := func(stage string, err error) {
		s.source.Complete, s.countsKnown = false, false
		diagnostic := fmt.Errorf("%s: %w", stage, err)
		if s.source.Diagnostic == "" {
			s.source.Diagnostic = diagnostic.Error()
		} else {
			s.source.Diagnostic = errors.Join(errors.New(s.source.Diagnostic), diagnostic).Error()
		}
	}
	read := func(in ports.AsyncActivityRead) ([]ports.AsyncActivityFact, bool, error) {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		in.Source, in.Limit = source, 500
		return reader.ReadAsyncActivityFacts(ctx, project, in)
	}
	projectFact := func(f ports.AsyncActivityFact) (api.AsyncActivity, error) {
		if err := ctx.Err(); err != nil {
			return api.AsyncActivity{}, err
		}
		return ProjectAsyncActivity(project, instance, f, now)
	}
	if !linksOnly {
		summary, more, err := read(ports.AsyncActivityRead{Filter: "summary"})
		if err != nil {
			fail("summary", err)
		}
		if more {
			s.countsKnown = false
		}
		for _, f := range summary {
			a, err := projectFact(f)
			if err != nil {
				fail("summary", err)
				if ctx.Err() != nil {
					break
				}
				continue
			}
			if activityTerminal(a.State) {
				s.counts["recent"]++
				s.counts["history"]++
			} else {
				s.counts["ongoing"]++
			}
			if a.NeedsAttention {
				s.counts["attention"]++
			}
		}
		s.rows, s.more, err = read(ports.AsyncActivityRead{Filter: o.Filter, AfterTime: o.afterTime, AfterID: o.afterID})
		if err != nil {
			fail("list", err)
		}
		if s.more {
			s.source.Complete, s.countsKnown = false, false
		}
		for _, f := range s.rows {
			a, err := projectFact(f)
			if err != nil {
				fail("list projection", err)
				if ctx.Err() != nil {
					break
				}
				continue
			}
			s.projected[activityKey(f)] = a
		}
	}
	if len(o.RequestIDs) > 0 {
		linked, more, err := read(ports.AsyncActivityRead{RequestIDs: o.RequestIDs})
		if err != nil {
			fail("links", err)
		}
		if more {
			fail("links", errors.New("activity association result incomplete"))
		}
		wanted := map[string]bool{}
		for _, id := range o.RequestIDs {
			wanted[id] = true
		}
		for _, f := range linked {
			a, err := projectFact(f)
			if err != nil {
				fail("links", err)
				if ctx.Err() != nil {
					break
				}
				continue
			}
			for _, id := range a.RequestIDs {
				if wanted[id] {
					s.links[id] = a
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		fail("source budget", err)
	}
	return s
}

func ReadAsyncActivityLinks(ctx context.Context, reader ports.AsyncActivityReader, project, instance string, o AsyncActivityOptions) (api.AsyncActivityLinks, error) {
	if len(o.RequestIDs) == 0 || len(o.RequestIDs) > 100 {
		return api.AsyncActivityLinks{}, errors.New("invalid activity association query")
	}
	out := api.AsyncActivityLinks{ProjectID: project, InstanceID: instance, SampledAt: time.Now().UTC(), Links: map[string]api.AsyncActivity{}}
	if o.PublicProjectID != "" {
		out.ProjectID = o.PublicProjectID
	}
	sources := o.Sources
	if len(sources) == 0 {
		sources = AsyncActivitySources
	}
	for _, sampled := range sampleActivitySources(ctx, reader, project, instance, sources, o, out.SampledAt, true) {
		out.Sources = append(out.Sources, sampled.source)
		for id, a := range sampled.links {
			a.ProjectID = out.ProjectID
			out.Links[id] = a
		}
	}
	return out, nil
}
