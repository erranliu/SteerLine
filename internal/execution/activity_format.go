package execution

import contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"

// decisionAnswerSummary is the execution audit's stable decision outcome text.
// Terminal coloring and width formatting live with product query/UI code.
func decisionAnswerSummary(run *contracts.Run) string {
	if run == nil || run.Decision == nil {
		return ""
	}
	d := run.Decision
	if d.OptionID == "" {
		return d.Note
	}
	for _, option := range d.Request.Options {
		if option.ID != d.OptionID {
			continue
		}
		if d.Note != "" {
			return d.OptionID + ": " + option.Label + " — " + d.Note
		}
		return d.OptionID + ": " + option.Label
	}
	return d.OptionID
}
