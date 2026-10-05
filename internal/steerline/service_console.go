package steerline

import (
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/console/activityrender"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os"
)

func (s *ServiceHost) Emit(ev ActivityEvent) {
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
		if ev.Source == "steer" {
			fmt.Fprintln(out, activityrender.RenderActivity(productquery.ActivityDisplayEvent(ev), activityrender.RenderOptions{Color: false}))
		}
	default:
		fmt.Fprintln(out, activityrender.RenderActivity(productquery.ActivityDisplayEvent(ev), activityrender.RenderOptions{Color: stdoutColor()}))
	}
}
