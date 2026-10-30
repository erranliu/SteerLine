package testenv

// Both Orca targets use the same public repo and terminal operations. Only the
// destination selector differs; handles and runtime identity stay bound there.
type orcaTarget struct {
	Profile                                            map[string]any
	Executable, Environment, Runtime, Repository, Root string
	Args                                               []string
	sequence                                           int
}

func (r *orcaTarget) commandArgs(args ...string) []string {
	result := append(append([]string{}, r.Args...), args...)
	if r.Environment != "" {
		result = append(result, "--environment", r.Environment)
	}
	return append(result, "--json")
}
