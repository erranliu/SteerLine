package host

import (
	"encoding/json"
	"fmt"
	"os"

	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
)

type ActivityFormatter interface {
	FormatActivity(executionvalues.ActivityEvent, bool) string
}

func (s *ServiceHost) Emit(ev executionvalues.ActivityEvent) {
	if s.Console == "" || s.Console == "off" {
		return
	}
	out := s.consoleOut
	if out == nil {
		out = os.Stdout
	}
	switch s.Console {
	case "json":

		s.consoleSequence++
		ev.V, ev.Seq = ActivityVersion, s.consoleSequence
		if ev.TS.IsZero() {
			ev.TS = now()
		}
		b, e := json.Marshal(ev)
		if e == nil {
			fmt.Fprintln(out, string(b))
		}
	case "summary":
		if ev.Source == "steer" && s.formatter != nil {
			fmt.Fprintln(out, s.formatter.FormatActivity(ev, false))
		}
	default:
		if s.formatter != nil {
			fmt.Fprintln(out, s.formatter.FormatActivity(ev, stdoutColor()))
		} else {
			fmt.Fprintf(out, "%s: %s %s\n", ev.Source, ev.Kind, ev.Title)
		}
	}
}
