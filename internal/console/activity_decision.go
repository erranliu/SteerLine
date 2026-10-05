package console

import api "github.com/erranliu/SteerLine/internal/product/api"

// DecisionAnswerSummary formats the saved answer for the audit journal and UI.
func DecisionAnswerSummary(r *api.CandidateSummary) string {
	if r.Decision == nil {
		return ""
	}
	if r.Decision.OptionID != "" {
		for _, o := range r.Decision.Request.Options {
			if o.ID == r.Decision.OptionID {
				if r.Decision.Note != "" {
					return r.Decision.OptionID + ": " + o.Label + " — " + r.Decision.Note
				}
				return r.Decision.OptionID + ": " + o.Label
			}
		}
		return r.Decision.OptionID
	}
	return r.Decision.Note
}
