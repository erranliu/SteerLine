package execution

import (
	"context"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	"sync"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type Service struct {
	replayDone           <-chan struct{}
	historyDone          <-chan struct{}
	historyRequests      chan []conversationtypes.ConversationHistorySource
	host                 HostPort
	taskWork             TaskWorkPort
	taskResults          TaskResultPort
	Config               Config
	baseConfig           Config
	lifecycleResolver    executionbackend.LifecycleResolver
	conversationSources  conversationtypes.ConversationSourcePort
	dispatchConfig       *Config
	dispatchProfile      *ExecutionProfile
	State                reconstruction.Runtime
	domain               reconstruction.Snapshot
	committed            reconstruction.Snapshot
	runs                 []*reconstruction.RunView
	pendingEvents        []storageports.DomainEvent
	pendingOperations    map[string]string
	workerDecisionWrites map[string]bool
	store                *Store
	domainStore          StateReader
	commandStore         CommandStore
	conversationStore    ConversationStore
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
	controlSeen      map[string]string
	stopControl      *productapi.ControlRequest
	stopAcceptedAt   *time.Time
	targetTask       string
	lastDispatched   string
	reservedRunID    string
	explicitDispatch bool
	controlFault     func(string)
	startupID        string
}
