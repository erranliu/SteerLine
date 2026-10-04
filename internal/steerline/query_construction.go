package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/query"
)

type QueryService = query.Service

type EventFilter = query.EventFilter

type EventPage = query.EventPage

type EventBodyPage = query.EventBodyPage

type CursorError = query.CursorError

const DefaultEventPageSize = query.DefaultEventPageSize

const MaxEventPageSize = query.MaxEventPageSize

const MaxEventPageBytes = query.MaxEventPageBytes

const eventReadBatchSize = query.EventReadBatchSize

func NewQueryService(store query.Store, project string) QueryService {
	return query.NewService(store, project)
}
