package steerline

import (
	"context"
	"io"
	"sync"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type Service struct {
	taskWork             TaskWorkPort
	taskResults          TaskResultPort
	Config               Config
	baseConfig           Config
	lifecycleResolver    executionbackend.LifecycleResolver
	conversationSources  contracts.ConversationSourcePort
	dispatchConfig       *Config
	dispatchProfile      *ExecutionProfile
	State                Runtime
	domain               DomainSnapshot
	committed            DomainSnapshot
	runs                 []*Run
	pendingEvents        []storageports.DomainEvent
	pendingOperations    map[string]string
	workerDecisionWrites map[string]bool
	store                *Store
	domainStore          centerReader
	commandStore         persistence.CommandStore
	conversationStore    persistence.Conversations
	closeStore           func() error
	// workers owns live observers by authoritative workflow run ID.
	workers                    map[string]*Worker
	ownedProcesses             map[string]executionbackend.StopHandle
	lifecycleEffects           map[string]pendingLifecycleEffect
	lifecycleMessages          map[string][]workerMessage
	appServerRecovering        map[string]bool
	workerMessages             chan workerMessage
	runContext                 context.Context
	forwarders                 sync.WaitGroup
	forwardMu                  sync.Mutex
	forwardedDone              []workerMessage
	conversationReplayRequests chan []conversationJournalSource
	conversationReplayErrors   chan error
	// Console selects service-side rendering: "", "off", "pretty", "summary" or "json".
	Console           string
	consoleOut        io.Writer
	consoleSequence   uint64
	controlSeen       map[string]string
	stopControl       *productapi.ControlRequest
	stopAcceptedAt    *time.Time
	targetTask        string
	lastDispatched    string
	reservedRunID     string
	candidateSnapshot *TaskSnapshot
	explicitDispatch  bool
	controlFault      func(string)
	startupID         string
}
