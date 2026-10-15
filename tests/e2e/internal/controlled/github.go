package controlled

import (
	"fmt"

	"os"

	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// Keep query evidence separate from the app-server's mutable state. A query
// must not overwrite a concurrent turn or publication receipt.
func recordProviderQuery(path string) error {
	f, err := os.OpenFile(path+".queries", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString("query\n")
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func ProviderQueryCount(path string) (int, error) {
	raw, err := os.ReadFile(path + ".queries")
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for _, line := range lines {
		if line != "query" {
			return 0, fmt.Errorf("incomplete provider query evidence")
		}
	}
	return len(lines), nil
}

func ProviderPR(s State, url string) (executionmodel.PRInfo, error) {
	if url == "" || url == s.PR.URL {
		if s.PR.URL == "" {
			return executionmodel.PRInfo{}, fmt.Errorf("controlled PR not published")
		}
		return s.PR, nil
	}
	if pr, ok := s.PRs[url]; ok {
		return pr, nil
	}
	return executionmodel.PRInfo{}, fmt.Errorf("unknown controlled PR URL")
}
