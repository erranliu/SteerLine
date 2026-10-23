package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTM05ProductDTO(t *testing.T) {
	t.Run("serializable_target_version_original_receipt", func(t *testing.T) {
		now := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
		input := ControlInput{Action: StartCandidate, RequestID: "original-request", InstanceID: "service-instance", ExpectedRevision: value(uint64(19)), Payload: ControlPayload{TaskID: "77", ExpectedTaskVersion: 4, ExpectedGraphRevision: 7}}
		receipt := ControlReceipt{RequestID: input.RequestID, InstanceID: input.InstanceID, RunID: "candidate-b", ControlRevision: 19, RunRevision: 9, Status: "accepted", AcceptedAt: &now}
		page := ProjectSnapshot{Connection: ServiceConnection{State: ConnectionConnected, Snapshot: ObservationSnapshot{Page: &ServicePage{Repository: RepositorySummary{Identity: "project"}, Runs: []*CandidateSummary{{TaskID: "77", RunID: "candidate-b", ExecutionID: "execution-b", AttemptID: "attempt-b", SessionID: "会话-b", ControlRevision: 9, EnvironmentBlocker: &EnvironmentBlocker{ID: "blocker", Version: 2}, Decision: &DecisionSummary{ID: "decision", Version: 3, Request: DecisionRequest{Question: "继续吗？"}}}}}}}}
		for _, original := range []any{input, receipt, page, &Error{Kind: Unsupported, Message: "source is unsupported"}, ActivityCursor{RunID: "candidate-b", ExecutionID: "execution-b", AttemptID: "attempt-b", AfterEventID: 45, Initialized: true}} {
			encoded, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			decoded := reflect.New(reflect.TypeOf(original))
			if err := json.Unmarshal(encoded, decoded.Interface()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, decoded.Elem().Interface()) {
				t.Fatalf("contract lost immutable identity or result: %s", encoded)
			}
		}
		if receipt.CompletedAt != nil || receipt.Status != "accepted" {
			t.Fatal("acceptance was promoted to a completed stop")
		}
	})
	t.Run("api_has_no_internal_models_aliases_or_go_handles", func(t *testing.T) {
		entries, err := os.ReadDir(".")
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range file.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(path, ".") {
					t.Fatalf("api imports implementation %s", path)
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if decl, ok := n.(*ast.TypeSpec); ok {
					if decl.Assign.IsValid() {
						t.Errorf("public alias %s", decl.Name.Name)
					}
					switch decl.Name.Name {
					case "Runtime", "DomainSnapshot", "Config", "TaskRecord", "TaskContract", "WorkflowRunRecord", "ExecutionRecord", "StepAttemptRecord", "DecisionRecord", "CommandRecord":
						t.Errorf("internal type exposed as %s", decl.Name.Name)
					}
				}
				if s, ok := n.(*ast.StructType); ok {
					for _, f := range s.Fields.List {
						switch f.Type.(type) {
						case *ast.FuncType, *ast.ChanType, *ast.InterfaceType:
							t.Errorf("Go handle in public DTO %s", entry.Name())
						}
						if ident, ok := f.Type.(*ast.Ident); ok && ident.Name == "error" {
							t.Errorf("unserializable error in %s", entry.Name())
						}
					}
				}
				return true
			})
		}
	})
	t.Run("task_page_escapes_bidi_preserves_unicode", func(t *testing.T) {
		page := TaskPage{Tasks: []TaskDetail{{TaskSummary: TaskSummary{TaskID: "77", Title: "中文😀\u202e标题"}, SpecPath: "SteerlineFlow/spec/固定.md"}}}
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "\u202e") || !strings.Contains(string(encoded), `\u202e`) {
			t.Fatalf("unsafe bidi output: %s", encoded)
		}
		var restored TaskPage
		if err := json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(restored, page) {
			t.Fatalf("Unicode changed: %v / %s", err, encoded)
		}
	})
}

func value[T any](v T) *T { return &v }
