package viewer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erranliu/SteerLine/internal/steerline"
)

const maxLogLines = 4000

// logMode selects what the bottom pane shows: the normalized journal, a
// conversation-shaped projection of it, or the raw agent stream.
type logMode int

const (
	modeActivity logMode = iota
	modeConversation
	modeRaw
)

func (m logMode) name() string {
	switch m {
	case modeConversation:
		return "conversation"
	case modeRaw:
		return "raw"
	}
	return "activity"
}

// Options configures the interactive viewer.
type Options struct {
	// RunID focuses a run by run id or task id on open.
	RunID string
	// Color enables color in rendered log lines.
	Color bool
}

type runRow struct {
	run     *steerline.Run
	pending bool
}

type modal struct {
	run       *steerline.Run
	choice    int
	note      string
	noteFocus bool
	err       string
}

// Model is the bubbletea model. It is a pure function of its state and the
// injected Source, which is what makes it testable with teatest.
type Model struct {
	cfg  steerline.Config
	src  Source
	opts Options

	width, height int
	rows          []runRow
	rt            *steerline.Runtime
	observation   steerline.ObservationSnapshot
	runsInFlight  bool
	sel           int
	offset        int
	focus         string
	selectedRunID string

	logRunID      string
	logPath       string
	logCursor     steerline.JournalCursor
	activeLogKey  string
	logRequestID  uint64
	logGeneration uint64
	inFlightLogs  map[string]uint64
	logError      string
	log           []steerline.ActivityEvent
	logScroll     int
	follow        bool
	autoScroll    bool
	mode          logMode
	raw           []string

	modal  *modal
	status string
	err    error

	// L2 detail pane state: selected tab and lazily-loaded provider task info.
	detailTab   int
	taskDetail  map[string]steerline.TaskDetail
	taskErr     map[string]string
	taskLoading map[string]bool
}

// NewModel builds the viewer model. src may be nil in tests that set it later.
func NewModel(c steerline.Config, src Source, o Options) Model {
	m := Model{
		cfg:          c,
		src:          src,
		opts:         o,
		follow:       true,
		autoScroll:   true,
		inFlightLogs: map[string]uint64{},
		taskDetail:   map[string]steerline.TaskDetail{},
		taskErr:      map[string]string{},
		taskLoading:  map[string]bool{},
	}
	if o.RunID != "" {
		m.focus = o.RunID
	}
	return m
}

// focus is applied once rows load.
func (m *Model) applyFocus() {
	if m.focus == "" {
		return
	}
	for i, r := range m.rows {
		if r.run.RunID == m.focus || r.run.TaskID == m.focus {
			m.sel = i
			break
		}
	}
	m.focus = ""
}

// IsInteractive reports whether both stdin and stdout are terminals, which is
// when the full-screen viewer makes sense. It uses only the standard library so
// the streaming path never depends on the TUI stack.
func IsInteractive() bool { return isTerminal(os.Stdin) && isTerminal(os.Stdout) }

func isTerminal(f *os.File) bool {
	info, e := f.Stat()
	return e == nil && info.Mode()&os.ModeCharDevice != 0
}

// Run starts the full-screen viewer. It returns when the user quits.
func Run(ctx context.Context, c steerline.Config, o Options) error {
	m := NewModel(c, NewFileSource(c), o)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, e := p.Run()
	return e
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return refreshRunsMsg{} }, tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) loadRuns() tea.Cmd {
	src := m.src
	return func() tea.Msg {
		obs := src.Snapshot()
		return runsMsg{observation: obs}
	}
}

func (m Model) startRunsRead() (Model, tea.Cmd) {
	if m.runsInFlight {
		return m, nil
	}
	m.runsInFlight = true
	return m, m.loadRuns()
}

func (m Model) readLog(logPath, runID string, key string, requestID uint64, cursor steerline.JournalCursor, raw bool) tea.Cmd {
	src := m.src
	generation := m.logGeneration
	return func() tea.Msg {
		batch, err := src.ReadLog(logPath, cursor, raw)
		return logMsg{runID: runID, logPath: logPath, key: key, requestID: requestID, generation: generation, base: cursor, batch: batch, err: err}
	}
}

func logKey(runID, path string, raw bool) string {
	mode := "activity"
	if raw {
		mode = "raw"
	}
	return runID + "\x00" + path + "\x00" + mode
}

func (m Model) clearLog() Model {
	m.logGeneration++
	m.logRunID, m.logPath, m.activeLogKey = "", "", ""
	m.logCursor = steerline.JournalCursor{}
	m.log, m.raw = nil, nil
	m.logError = ""
	m.logScroll, m.autoScroll = 0, true
	return m
}

// startLogRead starts at most one request for the selected stream. The cursor
// travels with the request so an old response cannot advance a newer stream.
func (m Model) startLogRead() (Model, tea.Cmd) {
	r := m.current()
	if r == nil || r.LogPath == nil || *r.LogPath == "" {
		return m.clearLog(), nil
	}
	path := *r.LogPath
	raw := m.mode == modeRaw
	key := logKey(r.RunID, path, raw)
	if m.logRunID != r.RunID || m.logPath != path || m.activeLogKey != key {
		m = m.clearLog()
		m.logRunID, m.logPath, m.activeLogKey = r.RunID, path, key
	}
	if m.inFlightLogs == nil {
		m.inFlightLogs = map[string]uint64{}
	}
	if _, busy := m.inFlightLogs[key]; busy {
		return m, nil
	}
	m.logRequestID++
	requestID := m.logRequestID
	m.inFlightLogs[key] = requestID
	return m, m.readLog(path, r.RunID, key, requestID, m.logCursor, raw)
}

func (m Model) loadTaskDetail(taskID string) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		d, e := src.TaskDetail(taskID)
		return taskDetailMsg{taskID: taskID, detail: d, err: e}
	}
}

// ensureTaskDetail starts one lazy, deduplicated `bd show` for the selected run.
// Failures are cached so a broken Beads install cannot spam the provider.
func (m Model) ensureTaskDetail() (Model, tea.Cmd) {
	r := m.current()
	if r == nil {
		return m, nil
	}
	id := r.TaskID
	if _, ok := m.taskDetail[id]; ok {
		return m, nil
	}
	if m.taskErr[id] != "" || m.taskLoading[id] {
		return m, nil
	}
	m.taskLoading[id] = true
	return m, m.loadTaskDetail(id)
}

func (m Model) answer(run *steerline.Run, optionID, note string) tea.Cmd {
	src := m.src
	by := steerline.AnswererName()
	return func() tea.Msg {
		return answerMsg{err: src.Answer(run, optionID, note, by)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m, runsCmd := m.startRunsRead()
		cmds := []tea.Cmd{runsCmd, tickCmd()}
		if m.follow {
			var cmd tea.Cmd
			m, cmd = m.startLogRead()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	case refreshRunsMsg:
		return m.startRunsRead()
	case runsMsg:
		m.runsInFlight = false
		previousID, previousIndex := m.selectedRunID, m.sel
		m.observation = msg.observation
		if msg.observation.Runtime != nil || msg.observation.State == steerline.SnapshotNone {
			m.rt = msg.observation.Runtime
		}
		m.observation.Runtime = m.rt
		if msg.observation.Error != "" {
			m.err = errors.New(msg.observation.Error)
		} else {
			m.err = nil
		}
		if m.rt != nil {
			m.rows = buildRows(m.rt.Runs)
		} else if msg.observation.State == steerline.SnapshotNone {
			m.rows = nil
		}
		if m.sel >= len(m.rows) {
			m.sel = len(m.rows) - 1
		}
		if m.sel < 0 {
			m.sel = 0
		}
		if previousID != "" {
			found := false
			for i, row := range m.rows {
				if row.run.RunID == previousID {
					m.sel, found = i, true
					break
				}
			}
			if !found && len(m.rows) > 0 {
				m.sel = previousIndex
				if m.sel >= len(m.rows) {
					m.sel = len(m.rows) - 1
				}
			}
		} else {
			m.applyFocus()
		}
		currentID := ""
		if r := m.current(); r != nil {
			currentID = r.RunID
		}
		pathChanged := false
		if r := m.current(); r != nil {
			if r.LogPath == nil || *r.LogPath == "" {
				pathChanged = m.logRunID == r.RunID && m.logPath != ""
			} else {
				pathChanged = currentID != m.logRunID || *r.LogPath != m.logPath || logKey(r.RunID, *r.LogPath, m.mode == modeRaw) != m.activeLogKey
			}
		}
		if currentID != previousID {
			m = m.clearLog()
			m.selectedRunID = currentID
		} else if currentID == "" {
			m = m.clearLog()
		}
		var cmds []tea.Cmd
		if currentID != "" && (currentID != previousID || pathChanged || m.activeLogKey == "") {
			var logCmd tea.Cmd
			m, logCmd = m.startLogRead()
			if logCmd != nil {
				cmds = append(cmds, logCmd)
			}
		}
		var cmd tea.Cmd
		m, cmd = m.ensureTaskDetail()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case taskDetailMsg:
		delete(m.taskLoading, msg.taskID)
		if msg.err != nil {
			m.taskErr[msg.taskID] = msg.err.Error()
		} else {
			m.taskDetail[msg.taskID] = msg.detail
		}
		return m, nil
	case logMsg:
		id, expected := m.inFlightLogs[msg.key]
		if expected && id == msg.requestID {
			delete(m.inFlightLogs, msg.key)
		}
		if expected && id == msg.requestID && msg.key == m.activeLogKey && msg.generation != m.logGeneration {
			// A previous visit still held this stream's in-flight slot. Discard
			// its result and only now start the newly selected visit's read.
			return m.startLogRead()
		}
		if !expected || id != msg.requestID || msg.key != m.activeLogKey || msg.runID != m.logRunID || msg.logPath != m.logPath || !sameCursor(msg.base, m.logCursor) {
			return m, nil
		}
		if msg.err != nil {
			m.logError = msg.err.Error()
			return m, nil
		}
		anchor, anchored := m.scrollAnchor()
		before := m.renderedLogLineCount()
		if msg.batch.Reset {
			m.log, m.raw = nil, nil
			m.logScroll, m.autoScroll = 0, true
			before = 0
		}
		m.log = append(m.log, msg.batch.Activity...)
		m.raw = append(m.raw, msg.batch.Raw...)
		unboundedAfter := m.renderedLogLineCount()
		if len(m.log) > maxLogLines {
			m.log = m.log[len(m.log)-maxLogLines:]
		}
		if len(m.raw) > maxLogLines {
			m.raw = m.raw[len(m.raw)-maxLogLines:]
		}
		m.logCursor = msg.batch.Cursor
		m.logError = strings.Join(msg.batch.Diagnostics, "; ")
		if m.autoScroll {
			m.logScroll = 0
		} else if !anchored || !m.restoreScrollAnchor(anchor) {
			if unboundedAfter > before {
				m.logScroll += unboundedAfter - before
			}
			if count := m.renderedLogLineCount(); m.logScroll > count {
				m.logScroll = count
			}
		}
		return m, nil
	case answerMsg:
		if msg.err != nil {
			if m.modal != nil {
				m.modal.err = msg.err.Error()
			}
			return m, nil
		}
		m.modal = nil
		m.status = "answer submitted; the service will resume the run"
		return m.startRunsRead()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// buildRows orders runs so items that need a human come first: pending
// decisions, then blocked/unknown executions, then active work, then the rest.
func buildRows(runs []*steerline.Run) []runRow {
	rows := make([]runRow, 0, len(runs))
	for _, r := range runs {
		if r == nil {
			continue
		}
		rows = append(rows, runRow{run: r, pending: isPending(r)})
	}
	sort.SliceStable(rows, func(i, j int) bool { return runRank(&rows[i]) < runRank(&rows[j]) })
	return rows
}

// runRank is the attention ordering key: lower is more urgent.
func runRank(r *runRow) int {
	if r == nil || r.run == nil {
		return 9
	}
	switch {
	case r.pending:
		return 0
	case r.run.FinishTaskStatus == "blocked" || r.run.ExecutionStatus == "unknown":
		return 1
	case r.run.ExecutionStatus == "running":
		return 2
	case r.run.FinishTaskStatus == "waiting_pr_merge":
		return 3
	case r.run.FinishTaskStatus == "completed":
		return 5
	}
	return 4
}

func isPending(r *steerline.Run) bool {
	return r != nil && r.FinishTaskStatus == "waiting_decision" && r.Decision != nil && r.Decision.Status == "pending"
}

func (m Model) current() *steerline.Run {
	if m.sel < 0 || m.sel >= len(m.rows) {
		return nil
	}
	return m.rows[m.sel].run
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		return m.handleModalKey(k)
	}
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		return m.move(-1)
	case "down", "j":
		return m.move(1)
	case "f":
		m.follow = !m.follow
		if m.follow {
			m.autoScroll = true
			m.logScroll = 0
			return m.startLogRead()
		}
		return m, nil
	case "v":
		wasRaw := m.mode == modeRaw
		m.mode = (m.mode + 1) % 3
		m.logScroll, m.autoScroll = 0, true
		m.status = "view: " + m.mode.name()
		if wasRaw != (m.mode == modeRaw) {
			m = m.clearLog()
			if r := m.current(); r != nil {
				m.selectedRunID = r.RunID
			}
			return m.startLogRead()
		}
		return m, nil
	case "g":
		m.logScroll = 1 << 30
		m.autoScroll = false
		return m, nil
	case "G":
		m.logScroll = 0
		m.autoScroll = true
		return m, nil
	case "pgup":
		m.logScroll += 5
		m.autoScroll = false
		return m, nil
	case "pgdown":
		if m.logScroll -= 5; m.logScroll < 0 {
			m.logScroll = 0
		}
		m.autoScroll = m.logScroll == 0
		return m, nil
	case "r":
		return m.startRunsRead()
	case "[":
		if m.detailTab > 0 {
			m.detailTab--
		}
		return m, nil
	case "]":
		if m.detailTab < detailTabCount-1 {
			m.detailTab++
		}
		return m, nil
	case "a", "enter":
		if r := m.current(); isPending(r) {
			m.modal = &modal{run: r, choice: preferredChoice(r)}
		}
		return m, nil
	}
	return m, nil
}

func preferredChoice(r *steerline.Run) int {
	if r == nil || r.Decision == nil {
		return 0
	}
	if rec := r.Decision.Request.Recommended; rec != "" {
		for i, o := range r.Decision.Request.Options {
			if o.ID == rec {
				return i
			}
		}
	}
	return 0
}

func (m Model) handleModalKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	mo := m.modal
	opts := mo.run.Decision.Request.Options
	switch k.String() {
	case "esc":
		m.modal = nil
		return m, nil
	case "tab", "shift+tab":
		mo.noteFocus = !mo.noteFocus
		return m, nil
	case "enter":
		optionID := ""
		if len(opts) > 0 {
			if mo.choice < 0 || mo.choice >= len(opts) {
				mo.choice = 0
			}
			optionID = opts[mo.choice].ID
		}
		mo.err = ""
		return m, m.answer(mo.run, optionID, mo.note)
	}
	if mo.noteFocus {
		switch k.String() {
		case "backspace":
			if len(mo.note) > 0 {
				mo.note = mo.note[:len(mo.note)-1]
			}
		case "ctrl+u":
			mo.note = ""
		default:
			if k.Type == tea.KeyRunes {
				mo.note += string(k.Runes)
			} else if k.Type == tea.KeySpace {
				mo.note += " "
			}
		}
		return m, nil
	}
	switch k.String() {
	case "up", "k":
		if mo.choice > 0 {
			mo.choice--
		}
	case "down", "j":
		if mo.choice < len(opts)-1 {
			mo.choice++
		}
	default:
		if k.Type == tea.KeyRunes && len(k.Runes) == 1 && k.Runes[0] >= '1' && k.Runes[0] <= '9' {
			if idx := int(k.Runes[0] - '1'); idx < len(opts) {
				mo.choice = idx
			}
		}
	}
	return m, nil
}

func (m Model) move(d int) (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, nil
	}
	old := m.sel
	m.sel += d
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(m.rows) {
		m.sel = len(m.rows) - 1
	}
	if m.sel == old {
		return m, nil
	}
	m.status = ""
	if r := m.current(); r != nil {
		m.selectedRunID = r.RunID
		m.logScroll, m.autoScroll = 0, true
		mm, cmd := m.ensureTaskDetail()
		mm, logCmd := mm.startLogRead()
		return mm, tea.Batch(logCmd, cmd)
	}
	m = m.clearLog()
	m.selectedRunID = ""
	mm, cmd := m.ensureTaskDetail()
	return mm, cmd
}

// --- messages ---

type tickMsg time.Time
type refreshRunsMsg struct{}
type runsMsg struct {
	observation steerline.ObservationSnapshot
}
type logMsg struct {
	runID      string
	logPath    string
	key        string
	requestID  uint64
	generation uint64
	base       steerline.JournalCursor
	batch      LogBatch
	err        error
}
type answerMsg struct{ err error }
type taskDetailMsg struct {
	taskID string
	detail steerline.TaskDetail
	err    error
}

func sameCursor(a, b steerline.JournalCursor) bool {
	return a.Path == b.Path && a.FileID == b.FileID && a.Generation == b.Generation && a.Offset == b.Offset && a.ScanOffset == b.ScanOffset && a.LastSeq == b.LastSeq && a.HaveSeq == b.HaveSeq && a.FallbackSeq == b.FallbackSeq && a.Dropping == b.Dropping && a.SkipUntilNewline == b.SkipUntilNewline && a.AnchorOffset == b.AnchorOffset && a.AnchorLength == b.AnchorLength && a.AnchorHash == b.AnchorHash && a.HeaderLength == b.HeaderLength && a.HeaderHash == b.HeaderHash && a.Initialized == b.Initialized && bytes.Equal(a.Pending, b.Pending)
}

func (m Model) renderedLogLineCount() int {
	if m.mode == modeRaw {
		return len(m.raw)
	}
	_, w := paneWidths(m.width)
	if w < 1 {
		w = 1
	}
	count := 0
	for _, ev := range foldEvents(m.log, m.mode == modeConversation) {
		count += len(strings.Split(steerline.RenderActivity(ev, steerline.RenderOptions{Color: m.opts.Color, MaxColumns: w}), "\n"))
	}
	return count
}

// truncate clips to a display width, not a rune count, so CJK titles cannot
// overflow their pane and push the layout off screen.
func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > max-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}
