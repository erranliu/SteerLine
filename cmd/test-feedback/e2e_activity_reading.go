package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func checkActivityReadingEvidence(dir string) error {
	var proof struct {
		Passed   bool                                            `json:"passed"`
		Summary  bool                                            `json:"summary_only_default"`
		DeepLink bool                                            `json:"deep_link_collapsed"`
		Index    bool                                            `json:"explicit_index_page"`
		NoAuto   bool                                            `json:"no_automatic_index_paging"`
		Body     bool                                            `json:"explicit_body_or_complete_preview"`
		History  bool                                            `json:"history_no_focus_refetch"`
		Keyboard bool                                            `json:"narrow_keyboard_navigation"`
		Widths   []int                                           `json:"viewport_widths"`
		Requests struct{ Indexes, Bodies, Changes, History int } `json:"requests"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, "browser", "activity-reading-proof.json"))
	if err != nil {
		return err
	}
	if json.Unmarshal(raw, &proof) != nil || !proof.Passed || !proof.Summary || !proof.DeepLink || !proof.Index || !proof.NoAuto || !proof.Body || !proof.History || !proof.Keyboard || len(proof.Widths) != 3 || proof.Widths[0] != 1440 || proof.Widths[1] != 768 || proof.Widths[2] != 320 || proof.Requests.Indexes != 1 || proof.Requests.Bodies < 0 || proof.Requests.Bodies > 1 || proof.Requests.Changes < 2 || proof.Requests.History < 1 {
		return errors.New("missing activity reading page demand-query or viewport proof")
	}
	return nil
}
