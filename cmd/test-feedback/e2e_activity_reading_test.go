package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestActivityReadingEvidenceRejectsMissingOrAutomaticPaging(t *testing.T) {
	dir := t.TempDir()
	if checkActivityReadingEvidence(dir) == nil {
		t.Fatal("missing evidence accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "browser"), 0700); err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{"passed": true, "summary_only_default": true, "deep_link_collapsed": true, "explicit_index_page": true, "no_automatic_index_paging": true, "explicit_body_or_complete_preview": true, "history_no_focus_refetch": true, "narrow_keyboard_navigation": true, "viewport_widths": []int{1440, 768, 320}, "requests": map[string]int{"indexes": 1, "bodies": 0, "changes": 2, "history": 1}}
	write := func() {
		raw, _ := json.Marshal(proof)
		if err := os.WriteFile(filepath.Join(dir, "browser", "activity-reading-proof.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := checkActivityReadingEvidence(dir); err != nil {
		t.Fatal(err)
	}
	proof["requests"].(map[string]int)["indexes"] = 2
	write()
	if checkActivityReadingEvidence(dir) == nil {
		t.Fatal("automatic index paging accepted")
	}
	proof["requests"].(map[string]int)["indexes"] = 1
	proof["deep_link_collapsed"] = false
	write()
	if checkActivityReadingEvidence(dir) == nil {
		t.Fatal("eager deep-link read accepted")
	}
	proof["deep_link_collapsed"] = true
	proof["narrow_keyboard_navigation"] = false
	write()
	if checkActivityReadingEvidence(dir) == nil {
		t.Fatal("missing narrow keyboard proof accepted")
	}
}
