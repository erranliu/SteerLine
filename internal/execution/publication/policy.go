package publication

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// CheckPolicy validates publication values without selecting an execution
// implementation. The application directory owns support decisions.
func CheckPolicy(p executionbackend.PublicationPolicy, provider Config) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := provider.ValidateRemote(strings.TrimSuffix(provider.BaseURL, "/") + "/" + provider.Repository + ".git"); err != nil {
		return errors.New("publication provider requires a valid kind/base_url/repository")
	}
	return nil
}

// Reject before persistence. Diagnostics deliberately never contain input.
func ValidateURL(raw string) error {
	if strings.ContainsAny(raw, "\r\n\x00") {
		return errors.New("publication URL contains invalid data [REDACTED]")
	}
	if !strings.Contains(raw, "://") {
		return nil
	} // Local paths and SCP SSH identities have no URL auth.
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("publication URL must not contain secrets or parameters [REDACTED]")
	}
	if u.User != nil {
		_, password := u.User.Password()
		if u.Scheme != "ssh" || password {
			return errors.New("publication URL credentials must use authentication references [REDACTED]")
		}
	}
	return nil
}

var secretArgument = regexp.MustCompile(`(?i)(token|password|passwd|secret|authorization|authentication|credential|extraheader|api[._-]?key|auth[._-]?key|access[_-]?key|private[_-]?key|bearer|^--?user(?:=|$)|^-u$)`)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ValidateNonSecret(c Config, git executionbackend.Command) error {
	if err := ValidateURL(c.BaseURL); err != nil {
		return err
	}
	if c.TokenEnv != "" && !envName.MatchString(c.TokenEnv) {
		return errors.New("authentication reference must be an environment variable name [REDACTED]")
	}
	for _, command := range []executionbackend.Command{c.Query, git} {
		for _, arg := range append([]string{command.Executable}, command.Args...) {
			if strings.ContainsAny(arg, "\r\n\x00") || secretArgument.MatchString(arg) {
				return errors.New("publication command must not embed authentication values; use references [REDACTED]")
			}
			// URL arguments may be nested in an option or expression.
			for _, field := range strings.FieldsFunc(arg, func(r rune) bool { return r == ' ' || r == '=' || r == '\'' || r == '"' }) {
				if strings.Contains(field, "://") {
					if err := ValidateURL(field); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func ValidateSettings(s executionmodel.PublicationSettings) error {
	if err := ValidateNonSecret(Config(s.Provider), s.Git); err != nil {
		return err
	}
	if err := ValidateURL(s.RemoteURL); err != nil {
		return err
	}
	for _, ref := range s.AuthenticationRefs {
		if !envName.MatchString(ref) {
			return errors.New("invalid fixed authentication reference")
		}
	}
	return nil
}
