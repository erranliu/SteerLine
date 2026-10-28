package model

import (
	"encoding/json"
	"strings"
	"testing"

	progresstools "github.com/erranliu/SteerLine/tools"
)

func TestInlineProgressWriterForProjectWithoutHelper(t *testing.T) {
	raw := []byte(`{"status":"ready","plan_path":"task.plan.md","commit":"fixed","definitions":[{"id":"one"}]}`)
	body := TestingContext(raw)
	if !strings.Contains(body, progresstools.PowerShellWriter) || !strings.Contains(body, "入口开始调用 Set-RuntimeProgress") || !strings.Contains(body, "逐项核对") {
		t.Fatal("missing complete writer or per-entry wiring contract")
	}
	if strings.Contains(TestingReadOnlyInstructions, progresstools.PowerShellWriter) || !strings.Contains(TestingReadOnlyInstructions, "不得写入") {
		t.Fatal("read-only boundary lost")
	}
}

func TestCallerPromptConvergenceCompactArrangement(t *testing.T) {
	full := Snapshot{Status: "ready", PlanPath: "task.plan.md", Commit: strings.Repeat("a", 40), Digest: strings.Repeat("b", 64), Definitions: []Definition{
		{ID: "z.last", Command: "original command", Note: strings.Repeat("长 note 必须保存在固定清单。", 100)},
		{ID: "a.first", Command: "second command", Note: strings.Repeat("独立约束", 100)},
	}}
	raw, _ := json.Marshal(full)
	original := string(raw)
	referenced := TestingContextWithWriter(raw, true)
	inline := TestingContext(raw)
	if !strings.Contains(referenced, `"tests":["z.last","a.first"]`) || !strings.Contains(referenced, full.Digest) || !strings.Contains(referenced, full.Commit) || strings.Contains(referenced, full.Definitions[0].Note) {
		t.Fatal("compact view changed IDs/order/identity or duplicated definitions")
	}
	if string(raw) != original || strings.Contains(referenced, progresstools.PowerShellWriter) || !strings.Contains(inline, progresstools.PowerShellWriter) {
		t.Fatal("snapshot or writer source changed")
	}
	beforeWriter := strings.Replace(ProgressWriterContext(false), "固定版本缺少或不同源的写入器不能直接沿用；将", "写入器缺失时将", 1)
	before := len("SteerLine 执行绑定测试安排（系统数据，不是文件路径）：\n" + string(raw) + "\n" + TestingInstructions + beforeWriter)
	if len(referenced) >= before || len(inline) >= before {
		t.Fatal("redundant long arrangement did not shrink")
	}
	t.Logf("UTF-8 arrangement sample: before=%d helper=%d no-helper=%d; full snapshot=%d unchanged", before, len(referenced), len(inline), len(raw))
}
