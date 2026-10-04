package execution

import (
	"context"
	"sync"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type Service struct {
	replayDone           <-chan struct{}
	historyDone          <-chan struct{}
	historyRequests      chan []contracts.ConversationHistorySource
	host                 HostPort
	taskWork             TaskWorkPort
	taskResults          TaskResultPort
	Config               Config
	baseConfig           Config
	lifecycleResolver    executionbackend.LifecycleResolver
	conversationSources  contracts.ConversationSourcePort
	dispatchConfig       *Config
	dispatchProfile      *ExecutionProfile
	State                contracts.Runtime
	domain               contracts.DomainSnapshot
	committed            contracts.DomainSnapshot
	runs                 []*contracts.Run
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
	lifecycleMessages          map[string][]Signal
	appServerRecovering        map[string]bool
	signalQueue                chan Signal
	runContext                 context.Context
	forwarders                 sync.WaitGroup
	forwardMu                  sync.Mutex
	forwardedDone              []Signal
	conversationReplayRequests chan []conversationJournalSource
	conversationReplayErrors   chan error
	// Console selects service-side rendering: "", "off", "pretty", "summary" or "json".
	controlSeen       map[string]string
	stopControl       *productapi.ControlRequest
	stopAcceptedAt    *time.Time
	targetTask        string
	lastDispatched    string
	reservedRunID     string
	candidateSnapshot *contracts.TaskSnapshot
	explicitDispatch  bool
	controlFault      func(string)
	startupID         string
}
