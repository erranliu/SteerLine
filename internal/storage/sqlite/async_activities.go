package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"strings"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

// Each source is sampled in one read statement. Its JSON contains domain facts,
// never UI state, and the query cannot access a writer or a Runner client.
func (s *SQLiteStore) ReadAsyncActivityFacts(ctx context.Context, project string, in ports.AsyncActivityRead) ([]ports.AsyncActivityFact, bool, error) {
	db, err := s.asyncActivityPool()
	if err != nil {
		return nil, false, err
	}
	var table, id, payload, related, created, finished, request string
	switch in.Source {
	case "environment_detection":
		table = "project_environments a"
		id = "a.activity_id"
		payload = "json_remove(CAST(a.payload AS TEXT),'$.prepared','$.execution','$.runner')"
		created = "json_extract(CAST(a.payload AS TEXT),'$.created_at')"
		finished = "(SELECT MAX(json_extract(r.value,'$.run.updated_at')) FROM json_each(CAST(a.payload AS TEXT),'$.rounds') r)"
		related = `json_object('current',EXISTS(SELECT 1 FROM project_environment_current c WHERE c.project_identity=a.project_identity AND json_extract(CAST(c.payload AS TEXT),'$.activity_id')=a.activity_id))`
		request = `EXISTS(SELECT 1 FROM environment_requests r WHERE r.project_identity=a.project_identity AND r.activity_id=a.activity_id AND r.request_id IN (%s))`
	case "baseline_check":
		table = "commands a"
		id = "a.command_id"
		payload = `json_object('command_id',a.command_id,'status',a.status,'payload',json(CAST(a.payload AS TEXT)),'created_at',a.created_at,'updated_at',a.updated_at,'result',CASE WHEN a.result IS NULL THEN NULL ELSE json(CAST(a.result AS TEXT)) END)`
		created = "a.created_at"
		finished = "CASE WHEN a.status IN ('applied','rejected') THEN a.updated_at ELSE NULL END"
		related = `json_object('current',a.command_id=(SELECT command_id FROM commands c WHERE c.project_identity=a.project_identity AND c.kind='project_maintenance' AND json_extract(CAST(c.payload AS TEXT),'$.action')='maintenance_check' ORDER BY c.created_at DESC,c.command_id DESC LIMIT 1))`
		request = "a.command_id IN (%s)"
	case "test_preparation":
		table = "preparation_batches a"
		id = "a.batch_id"
		payload = "CAST(a.payload AS TEXT)"
		created = "json_extract(CAST(a.payload AS TEXT),'$.created_at')"
		finished = `MAX(COALESCE(json_extract(CAST(a.payload AS TEXT),'$.business_finished_at'),''),COALESCE((SELECT MAX(json_extract(CAST(g.payload AS TEXT),'$.report.updated_at')) FROM preparation_associations x JOIN test_generations g ON g.project_identity=x.project_identity AND g.generation_id=x.generation_id WHERE x.project_identity=a.project_identity AND x.batch_id=a.batch_id),''))`
		related = `json_object('current',EXISTS(SELECT 1 FROM maintenance_settings s WHERE s.project_identity=a.project_identity AND s.generation_batch=a.batch_id),
		'generations',json((SELECT COALESCE(json_group_array(json_object('id',x.generation_id,'inputs',x.inputs,'present',g.generation_id IS NOT NULL,'state',json_extract(CAST(g.payload AS TEXT),'$.state'),'submission',json_extract(CAST(g.payload AS TEXT),'$.submission'),'cancelled',json_extract(CAST(g.payload AS TEXT),'$.cancelled'),'run_id',json_extract(CAST(g.payload AS TEXT),'$.run_id'),'stop_status',json_extract(CAST(g.payload AS TEXT),'$.report.stop_status'),'run_status',json_extract(CAST(g.payload AS TEXT),'$.report.status'),'run_phase',json_extract(CAST(g.payload AS TEXT),'$.report.phase'),'diagnostic',COALESCE(json_extract(CAST(g.payload AS TEXT),'$.diagnostic'),''),'updated_at',json_extract(CAST(g.payload AS TEXT),'$.report.updated_at'),'observed_at',NULLIF(o.last_success,''),'observation_diagnostic',COALESCE(o.diagnostic,''),'relevant',EXISTS(SELECT 1 FROM project_materials m WHERE m.project_identity=x.project_identity AND m.task_id=x.task_id AND json_extract(CAST(m.payload AS TEXT),'$.inputs')=x.inputs AND json_extract(CAST(m.payload AS TEXT),'$.generation_id')=x.generation_id))), '[]') FROM preparation_associations x LEFT JOIN test_generations g ON g.project_identity=x.project_identity AND g.generation_id=x.generation_id LEFT JOIN source_observations o ON o.project_identity=x.project_identity AND o.source='test_generation' AND o.source_id=x.generation_id WHERE x.project_identity=a.project_identity AND x.batch_id=a.batch_id)),
		'requests',json((SELECT COALESCE(json_group_array(r.request_id),'[]') FROM maintenance_requests r WHERE r.project_identity=a.project_identity AND json_extract(CAST(r.result AS TEXT),'$.id')=a.batch_id)))`
		request = `EXISTS(SELECT 1 FROM maintenance_requests r WHERE r.project_identity=a.project_identity AND json_extract(CAST(r.result AS TEXT),'$.id')=a.batch_id AND r.request_id IN (%s))`
	case "task_candidate":
		table = "runner_candidates a"
		id = "a.candidate_id"
		payload = "json_remove(CAST(a.payload AS TEXT),'$.contract','$.submit','$.test_plan','$.runner_build','$.environment')"
		created = "json_extract(CAST(a.payload AS TEXT),'$.created_at')"
		finished = "json_extract(CAST(a.payload AS TEXT),'$.projection.updated_at')"
		related = `json_object('task_status',(SELECT t.status FROM tasks t WHERE t.project_identity=a.project_identity AND t.task_id=a.task_id),'title',(SELECT json_extract(CAST(t.payload AS TEXT),'$.title') FROM tasks t WHERE t.project_identity=a.project_identity AND t.task_id=a.task_id),'winner',(SELECT d.selected_workflow_run_id FROM task_deliveries d WHERE d.project_identity=a.project_identity AND d.task_id=a.task_id),'progress',json((SELECT CAST(p.payload AS TEXT) FROM candidate_progress p WHERE p.project_identity=a.project_identity AND p.candidate_id=a.candidate_id)))`
		request = "a.candidate_id IN (%s)"
	default:
		return nil, false, fmt.Errorf("unknown activity source %q", in.Source)
	}
	related = "json_set(" + related + ",'$.observation',json((SELECT json_object('observed_at',NULLIF(o.last_success,''),'diagnostic',o.diagnostic) FROM source_observations o WHERE o.project_identity=a.project_identity AND o.source='" + in.Source + "' AND o.source_id=" + id + ")))"
	args := []any{project}
	where := "a.project_identity=?"
	if in.Source == "baseline_check" {
		where += " AND a.kind='project_maintenance' AND json_extract(CAST(a.payload AS TEXT),'$.action')='maintenance_check'"
	}
	// Empty-status synthetic preparation groups are not runnable preparations.
	if in.Source == "test_preparation" {
		where += " AND COALESCE(json_extract(CAST(a.payload AS TEXT),'$.status'),'')<>''"
	}

	if len(in.RequestIDs) == 0 && in.Filter != "summary" {
		terminal := "COALESCE(json_extract(CAST(a.payload AS TEXT),'$.state'),'') IN ('ready','failed','cancelled')"
		switch in.Source {
		case "environment_detection":
			unsettled := "EXISTS(SELECT 1 FROM json_each(CAST(a.payload AS TEXT),'$.rounds') r WHERE COALESCE(json_extract(r.value,'$.settled'),0)=0)"
			if in.Filter == "recent" {
				where += " AND (" + terminal + ")"
			} else if in.Filter == "ongoing" {
				where += " AND (NOT (" + terminal + ") OR " + unsettled + ")"
			}
		case "baseline_check":
			if in.Filter == "recent" {
				where += " AND a.status IN ('applied','rejected')"
			} else if in.Filter == "ongoing" {
				where += " AND a.status NOT IN ('applied','rejected')"
			}
		case "test_preparation":
			if in.Filter == "recent" {
				where += " AND json_extract(CAST(a.payload AS TEXT),'$.status') IN ('completed','failed')"
			}
		case "task_candidate":
			if in.Filter == "recent" {
				where += " AND (json_extract(CAST(a.payload AS TEXT),'$.participation')<>'active' OR json_extract(CAST(a.payload AS TEXT),'$.submit_state') IN ('rejected','cancelled') OR json_extract(CAST(a.payload AS TEXT),'$.projection.status') IN ('failed','completed','closed','stopped'))"
			} else {
				where += " AND (COALESCE(json_type(CAST(a.payload AS TEXT),'$.projection'),'null')='null' OR json_extract(CAST(a.payload AS TEXT),'$.participation')='active' OR json_extract(CAST(a.payload AS TEXT),'$.stop_requested')=1 AND COALESCE(json_extract(CAST(a.payload AS TEXT),'$.projection.stop_status'),'')<>'stopped' OR json_extract(CAST(a.payload AS TEXT),'$.submit_state')='unknown' OR EXISTS(SELECT 1 FROM json_each(CAST(a.payload AS TEXT),'$.actions') action WHERE json_extract(action.value,'$.state')='unknown' OR json_extract(action.value,'$.state')='accepted' AND COALESCE(json_extract(action.value,'$.action_status'),'') NOT IN ('completed','failed','rejected','cancelled')))"
			}
		}
	}
	if len(in.RequestIDs) > 0 {
		if len(in.RequestIDs) > 100 {
			return nil, false, fmt.Errorf("too many request IDs")
		}
		where += " AND " + fmt.Sprintf(request, strings.TrimSuffix(strings.Repeat("?,", len(in.RequestIDs)), ","))
		for _, r := range in.RequestIDs {
			args = append(args, r)
		}
	}
	sortAt := created
	desc := in.Filter == "recent"
	if desc {
		sortAt = "COALESCE(" + finished + ",'')"
	} else {
		sortAt = "COALESCE(" + created + ",'')"
	}
	cmp, order := ">", "ASC"
	if desc {
		cmp, order = "<", "DESC"
	}
	if in.Limit < 1 || in.Limit > 500 {
		return nil, false, fmt.Errorf("activity source limit outside 1..500")
	}
	query := `SELECT id,sort_at,payload,related FROM (SELECT ` + id + ` AS id,` + sortAt + ` AS sort_at,` + payload + ` AS payload,` + related + ` AS related FROM ` + table + ` WHERE ` + where + `)`
	if in.AfterID != "" {
		query += " WHERE sort_at " + cmp + " ? OR (sort_at=? AND '" + in.Source + "/'||id>?)"
		args = append(args, in.AfterTime, in.AfterTime, in.AfterID)
	}
	query += " ORDER BY sort_at " + order + ",id ASC LIMIT ?"
	args = append(args, in.Limit+1)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := []ports.AsyncActivityFact{}
	bytesRead := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var f ports.AsyncActivityFact
		f.Source = in.Source
		var payload, related string
		if err = rows.Scan(&f.ID, &f.SortAt, &payload, &related); err != nil {
			return nil, false, err
		}
		size := len(payload) + len(related)
		if size > 2<<20 {
			return nil, false, fmt.Errorf("activity source fact exceeds observation byte budget")
		}
		if bytesRead+size > 2<<20 && len(result) > 0 {
			return result, true, nil
		}
		bytesRead += size
		f.Payload = []byte(payload)
		f.Related = []byte(related)
		// Domain methods are evaluated at their existing owner boundary. The
		// product query receives their facts without importing domain services.
		var relation map[string]any
		if err = json.Unmarshal(f.Related, &relation); err != nil {
			return nil, false, err
		}
		if in.Source == "environment_detection" {
			var record env.Record
			if err = json.Unmarshal(f.Payload, &record); err != nil {
				return nil, false, err
			}
			relation["settled"] = record.Settled()
			relation["domain_diagnostic"] = record.DiagnosticText()
		}
		if in.Source == "test_preparation" {
			var relationValues struct {
				Generations []struct {
					State, Submission  string
					Present, Cancelled int
					RunID              string `json:"run_id"`
					StopStatus         string `json:"stop_status"`
					RunStatus          string `json:"run_status"`
					RunPhase           string `json:"run_phase"`
				}
			}
			if err = json.Unmarshal(f.Related, &relationValues); err != nil {
				return nil, false, err
			}
			if values, ok := relation["generations"].([]any); ok {
				for i, g := range relationValues.Generations {
					record := tm.Generation{State: g.State, Submission: g.Submission, Cancelled: g.Cancelled == 1, RunID: g.RunID}
					record.Projection = &runnercontract.Run{Status: g.RunStatus, Phase: g.RunPhase, StopStatus: g.StopStatus}
					values[i].(map[string]any)["settled"] = g.Present == 1 && record.Settled()
					values[i].(map[string]any)["needs_attention"] = g.Present == 1 && record.NeedsAttention()
				}
			}
		}
		f.Related, err = json.Marshal(relation)
		if err != nil {
			return nil, false, err
		}
		result = append(result, f)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(result) > in.Limit
	if more {
		result = result[:in.Limit]
	}
	return result, more, nil
}
