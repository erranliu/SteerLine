package testenv

import (
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"path/filepath"
)

func (p *Project) adoptFixtureMaterials() {
	s := p.Scenario
	client, e := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(e)
	defer client.Close()
	_, err := driver.WaitFixtureMaterials(s.Context, p.Home, client.Identity(), "docs/spec/task.plan.md", filepath.Join(s.Output, p.Name+"-maintenance"))
	s.Must(err)
}
