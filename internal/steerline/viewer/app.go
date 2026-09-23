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
	sel           int
	offset        int
	focus         string

	loadedLogFor string
	log          []steerline.ActivityEvent
	logFrom      int64
	logScroll    int
	follow       bool

	modal  *modal
	status string
	err    error
}

// NewModel builds the viewer model. src may be nil in tests that set it later.
func NewModel(c steerline.Config, src Source, o Options) Model {
	m := Model{cfg: c, src: src, opts: o, follow: true}
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
		runs, e := src.Runs()
		return runsMsg{runs: runs, err: e}
	}
}

func (m Model) loadActivity(logPath string, from int64) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		ev, next, e := src.Activity(logPath, from)
		return activityMsg{logPath: logPath, events: ev, from: next, err: e}
	}
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
				cmds = append(cmds, m.loadActivity(*r.LogPath, m.logFrom))
			}
		}
		return m, tea.Batch(cmds...)
	case runsMsg:
		m.err = msg.err
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
			m.log, m.logFrom, m.logScroll = nil, 0, 0
			cmds = append(cmds, m.loadActivity(*r.LogPath, 0))
		}
		return m, tea.Batch(cmds...)
	case activityMsg:
		if r := m.current(); r != nil && r.LogPath != nil && *r.LogPath == msg.logPath {
			m.log = append(m.log, msg.events...)
			if len(m.log) > maxLogLines {
				m.log = m.log[len(m.log)-maxLogLines:]
			}
			m.logFrom = msg.from
			if m.follow {
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

func buildRows(runs []*steerline.Run) []runRow {
	rows := make([]runRow, 0, len(runs))
	for _, r := range runs {
		if r == nil {
			continue
		}
		rows = append(rows, runRow{run: r, pending: isPending(r)})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].pending && !rows[j].pending })
	return rows
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
	case "pgup":
		m.logScroll += 5
		return m, nil
	case "pgdown":
		if m.logScroll -= 5; m.logScroll < 0 {
			m.logScroll = 0
		}
		return m, nil
	case "f":
		m.follow = !m.follow
		return m, nil
	case "r":
		return m, m.loadRuns()
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
		m.log, m.logFrom, m.logScroll = nil, 0, 0
		return m, m.loadActivity(*r.LogPath, 0)
	}
	return m, nil
}

// --- messages ---

type tickMsg time.Time
type runsMsg struct {
	runs []*steerline.Run
	err  error
}
type activityMsg struct {
	logPath string
	events  []steerline.ActivityEvent
	from    int64
	err     error
}
type answerMsg struct{ err error }

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
