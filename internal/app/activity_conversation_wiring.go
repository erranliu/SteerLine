package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/erranliu/SteerLine/internal/activityconversation"
	"github.com/erranliu/SteerLine/internal/environmentadaptation"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/projectmaintenance"
	"github.com/erranliu/SteerLine/internal/refactoring"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/roadmap"
	roadmapmodel "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	advisorycontract "github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
	advisory "github.com/erranliu/SteerLine/internal/steeragentbridge/suggestion"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type activityConversationReader func(context.Context, string, string) (activityconversation.Facts, error)

type activityConversationStore interface {
	Candidate(context.Context, string, string) (runnerstate.Candidate, error)
	Environment(context.Context, string, string) (env.Record, error)
	Roadmap(context.Context, string, string) (roadmapmodel.Record, error)
	Refactoring(context.Context, string, string) (ref.Record, error)
	LoadAdvisory(context.Context, string, string) ([]byte, uint64, error)
	LookupCommand(context.Context, string, string) (ports.CommandRecord, bool, error)
}

func newActivityConversationReaders(store activityConversationStore) map[string]activityConversationReader {
	return map[string]activityConversationReader{
		"task_candidate":        bindActivityConversation(store.Candidate, ports.ErrNotFound, taskrunner.CandidateConversation),
		"environment_detection": bindActivityConversation(store.Environment, env.ErrNotFound, environmentadaptation.Conversation),
		"roadmap":               bindActivityConversation(store.Roadmap, roadmapmodel.ErrNotFound, roadmap.Conversation),
		"refactoring":           bindActivityConversation(store.Refactoring, ref.ErrNotFound, refactoring.Conversation),
		"main_agent_advisory": func(ctx context.Context, project, id string) (activityconversation.Facts, error) {
			facts, err := advisory.Conversation(ctx, store.LoadAdvisory, project, id)
			if errors.Is(err, advisorycontract.ErrNotFound) {
				err = sql.ErrNoRows
			}
			return facts, err
		},
		"baseline_check": func(ctx context.Context, project, id string) (activityconversation.Facts, error) {
			record, found, err := store.LookupCommand(ctx, project, id)
			if err != nil {
				return activityconversation.Facts{}, err
			}
			if !found {
				return activityconversation.Facts{}, sql.ErrNoRows
			}
			facts, ok := projectmaintenance.BaselineConversation(id, record.Kind, record.Status, record.Payload, record.Result)
			if !ok {
				return activityconversation.Facts{}, sql.ErrNoRows
			}
			return facts, nil
		},
	}
}

func bindActivityConversation[T any](read func(context.Context, string, string) (T, error), missing error, projectRecord func(T) activityconversation.Facts) activityConversationReader {
	return func(ctx context.Context, project, id string) (activityconversation.Facts, error) {
		record, err := read(ctx, project, id)
		if errors.Is(err, missing) {
			return activityconversation.Facts{}, sql.ErrNoRows
		}
		if err != nil {
			return activityconversation.Facts{}, err
		}
		return projectRecord(record), nil
	}
}
