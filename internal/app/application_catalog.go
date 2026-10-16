package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
)

// applicationCatalog is a frozen file census with no runtime or cancellation
// capability. Stop verification uses it directly without a live directory.
type catalogProject struct {
	view contract.Project
	ref  api.ProjectRef
	data []byte
}
type applicationCatalog struct {
	entries []*catalogProject
	global  []byte
	err     error
}

func loadApplicationCatalog(home string) *applicationCatalog {
	d := &applicationCatalog{}
	d.global, _ = readConfiguration(filepath.Join(home, "execution.json"))
	f, err := os.Open(filepath.Join(home, "etc"))
	if os.IsNotExist(err) {
		return d
	}
	if err != nil {
		d.err = err
		return d
	}
	defer f.Close()
	files, err := f.ReadDir(513)
	if err != nil && err != io.EOF {
		d.err = err
		return d
	}
	if len(files) > 512 {
		d.err = errors.New("project directory exceeds 512 entries")
		return d
	}
	for _, file := range files {
		if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".json") {
			continue
		}
		entry := &catalogProject{view: contract.Project{ConfigID: file.Name(), Name: strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())), RuntimeState: "starting"}, ref: api.ProjectRef{ConfigPath: filepath.Join(home, "etc", file.Name()), ApplicationHome: home}}
		entry.data, err = readConfiguration(entry.ref.ConfigPath)
		if err != nil {
			entry.view.RuntimeState = "failed"
			entry.view.Diagnostic = boundedDiagnostic(err.Error(), 4096)
		}
		d.entries = append(d.entries, entry)
	}
	sort.Slice(d.entries, func(i, j int) bool { return d.entries[i].view.ConfigID < d.entries[j].view.ConfigID })
	return d
}

func readConfiguration(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("configuration exceeds 1 MiB")
	}
	return data, nil
}
