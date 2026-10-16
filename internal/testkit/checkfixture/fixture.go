// Package checkfixture contains provisional public contract data, not a native
// simulator. It must be replaced/checked against Runner's published handoff.
package checkfixture

import (
	"embed"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

//go:embed v1.json
var assets embed.FS

type Fixture struct {
	Source         string                         `json:"source"`
	ResourceDigest string                         `json:"resource_digest"`
	Capabilities   runnerclient.CheckCapabilities `json:"capabilities"`
	Input          runnerclient.CheckInput        `json:"input"`
	Receipt        runnerclient.CheckReceipt      `json:"receipt"`
	Inspection     runnerclient.CheckInspection   `json:"inspection"`
}

func Load() Fixture {
	var f Fixture
	b, err := assets.ReadFile("v1.json")
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(b, &f); err != nil {
		panic(err)
	}
	f.Inspection.Check.Context = f.Input
	return f
}
