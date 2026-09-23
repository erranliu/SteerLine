package viewer

import (
	"context"
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
	sel           int
	offset        int
	focus         string

	loadedLogFor string
	log          []steerline.ActivityEvent
	logFrom      int64
	logScroll    int
	follow       bool
	autoScroll   bool
	mode         logMode
	raw          []string
	rawFrom      int64

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
		cfg:         c,
		src:         src,
		opts:        o,
		follow:      true,
		autoScroll:  true,
		taskDetail:  map[string]steerline.TaskDetail{},
		taskErr:     map[string]string{},
		taskLoading: map[string]bool{},
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
	return tea.Batch(m.loadRuns(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) loadRuns() tea.Cmd {
	src := m.src
	return func() tea.Msg {
		rt, e := src.Snapshot()
		if e != nil {
			return runsMsg{err: e}
		}
		if rt == nil {
			return runsMsg{}
		}
		return runsMsg{runs: rt.Runs, rt: rt}
	}
}

func (m Model) loadActivity(logPath string, from int64) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		ev, next, e := src.Activity(logPath, from)
		return activityMsg{logPath: logPath, events: ev, from: next, err: e}
	}
}

func (m Model) loadRaw(logPath string, from int64) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		lines, next, e := src.Events(logPath, from)
		return rawMsg{logPath: logPath, lines: lines, from: next, err: e}
	}
}

// loadLog picks the right reader for the active mode and resets the buffer.
func (m Model) loadLog(logPath string) tea.Cmd {
	if m.mode == modeRaw {
		m.raw, m.rawFrom = nil, 0
		return m.loadRaw(logPath, 0)
	}
	m.log, m.logFrom = nil, 0
	return m.loadActivity(logPath, 0)
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
		cmds := []tea.Cmd{m.loadRuns(), tickCmd()}
		if m.follow {
			if r := m.current(); r != nil && r.LogPath != nil {
				if m.mode == modeRaw {
					cmds = append(cmds, m.loadRaw(*r.LogPath, m.rawFrom))
				} else {
					cmds = append(cmds, m.loadActivity(*r.LogPath, m.logFrom))
				}
			}
		}
		return m, tea.Batch(cmds...)
	case runsMsg:
		m.err = msg.err
		m.rt = msg.rt
		m.rows = buildRows(msg.runs)
		if m.sel >= len(m.rows) {
			m.sel = len(m.rows) - 1
		}
		if m.sel < 0 {
			m.sel = 0
		}
		m.applyFocus()
		var cmds []tea.Cmd
		if r := m.current(); r != nil && r.LogPath != nil && m.loadedLogFor != r.RunID {
			m.loadedLogFor = r.RunID
			m.logScroll, m.autoScroll = 0, true
			cmds = append(cmds, m.loadLog(*r.LogPath))
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
	case activityMsg:
		if r := m.current(); r != nil && r.LogPath != nil && *r.LogPath == msg.logPath {
			m.log = append(m.log, msg.events...)
			if len(m.log) > maxLogLines {
				m.log = m.log[len(m.log)-maxLogLines:]
			}
			m.logFrom = msg.from
			if m.autoScroll {
				m.logScroll = 0
			}
		}
		if msg.err != nil && !os.IsNotExist(msg.err) {
			m.err = msg.err
		}
		return m, nil
	case rawMsg:
		if r := m.current(); r != nil && r.LogPath != nil && *r.LogPath == msg.logPath {
			m.raw = append(m.raw, msg.lines...)
			if len(m.raw) > maxLogLines {
				m.raw = m.raw[len(m.raw)-maxLogLines:]
			}
			m.rawFrom = msg.from
			if m.autoScroll {
				m.logScroll = 0
			}
		}
		if msg.err != nil && !os.IsNotExist(msg.err) {
			m.err = msg.err
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
		return m, m.loadRuns()
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
		}
		return m, nil
	case "v":
		prev := m.mode
		m.mode = (m.mode + 1) % 3
		m.logScroll, m.autoScroll = 0, true
		m.status = "view: " + m.mode.name()
		if r := m.current(); r != nil && r.LogPath != nil {
			if m.mode == modeRaw || prev == modeRaw {
				return m, m.loadLog(*r.LogPath)
			}
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
		return m, m.loadRuns()
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
	if r := m.current(); r != nil && r.LogPath != nil {
		m.loadedLogFor = r.RunID
		m.logScroll, m.autoScroll = 0, true
		mm, cmd := m.ensureTaskDetail()
		return mm, tea.Batch(m.loadLog(*r.LogPath), cmd)
	}
	mm, cmd := m.ensureTaskDetail()
	return mm, cmd
}

// --- messages ---

type tickMsg time.Time
type runsMsg struct {
	runs []*steerline.Run
	rt   *steerline.Runtime
	err  error
}
type activityMsg struct {
	logPath string
	events  []steerline.ActivityEvent
	from    int64
	err     error
}
type rawMsg struct {
	logPath string
	lines   []string
	from    int64
	err     error
}
type answerMsg struct{ err error }
type taskDetailMsg struct {
	taskID string
	detail steerline.TaskDetail
	err    error
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
