package steerline

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestRealIsolatedRecoveryPreview(t *testing.T) {
	configPath, runID := os.Getenv("STEERLINE_REAL_RECOVERY_CONFIG"), os.Getenv("STEERLINE_REAL_RECOVERY_RUN_ID")
	if configPath == "" || runID == "" {
		t.Skip("set STEERLINE_REAL_RECOVERY_CONFIG and STEERLINE_REAL_RECOVERY_RUN_ID for an isolated read-only preview")
	}
	c, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := InspectRecovery(context.Background(), c, runID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read-only recovery preview: %s", data)
}
