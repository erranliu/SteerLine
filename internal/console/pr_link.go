package console

import (
	"net/url"
	"unicode"
)

// Only the console supplies terminal control sequences; source text stays escaped.
func prLink(raw, label string) string {
	label = safeTerminalInlineText(label)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil {
		return label
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) || isBidiControl(r) {
			return label
		}
	}
	// ST termination is preserved by the ANSI-aware layout/truncation helpers.
	return "\x1b]8;;" + raw + "\x1b\\" + label + "\x1b]8;;\x1b\\"
}
