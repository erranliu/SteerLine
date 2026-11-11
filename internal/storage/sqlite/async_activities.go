package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
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
	case "refactoring":
		table = "refactoring_actions a"
		id = "a.action_id"
		payload = "json_remove(CAST(a.payload AS TEXT),'$.frozen','$.operations','$.answers','$.pending_prompt','$.reports','$.candidate')"
		created = "json_extract(a.payload,'$.created_at')"
		finished = "json_extract(a.payload,'$.finished_at')"
		related = `json_object('requests',json((SELECT COALESCE(json_group_array(c.command_id),'[]') FROM commands c WHERE c.project_identity=a.project_identity AND c.kind='refactoring' AND json_extract(c.result,'$.id')=a.action_id)))`
		request = `EXISTS(SELECT 1 FROM commands c WHERE c.project_identity=a.project_identity AND c.kind='refactoring' AND json_extract(c.result,'$.id')=a.action_id AND c.command_id IN (%s))`
	case "main_agent_advisory":
		table = "advisory_jobs a"
		id = "a.request_id"
		payload = "json_remove(CAST(a.payload AS TEXT),'$.request','$.suggestion','$.source.facts','$.source.goal','$.prepared_text','$.runner_request','$.runner','$.projection','$.stop_request')"
		created = "json_extract(a.payload,'$.created_at')"
		finished = "json_extract(a.payload,'$.finished_at')"
		related = "'{}'"
		request = "a.request_id IN (%s)"
	case "environment_detection":
		table = "project_environments a"
		id = "a.activity_id"
		payload = `json_object('id',a.activity_id,'project',a.project_identity,'revision',a.revision,
		'kind',json_extract(a.payload,'$.kind'),'state',json_extract(a.payload,'$.state'),'profile_id',json_extract(a.payload,'$.profile_id'),
		'observed_at',json_extract(a.payload,'$.observed_at'),'observation_diagnostic',json_extract(a.payload,'$.observation_diagnostic'),
		'created_at',json_extract(a.payload,'$.created_at'),'diagnostic',json_extract(a.payload,'$.diagnostic'),
		'cancelled',json(CASE WHEN json_extract(a.payload,'$.cancelled') THEN 'true' ELSE 'false' END),
		'requests',json((SELECT COALESCE(json_group_array(json_object('request_id',json_extract(q.value,'$.request_id'),'action',json_extract(q.value,'$.action'))),'[]') FROM json_each(a.payload,'$.requests') q)),
		'rounds',json((SELECT COALESCE(json_group_array(json_object(
		'number',json_extract(r.value,'$.number'),'run_id',json_extract(r.value,'$.run_id'),
		'settled',json(CASE WHEN json_extract(r.value,'$.settled') THEN 'true' ELSE 'false' END),
		'timed_out',json(CASE WHEN json_extract(r.value,'$.timed_out') THEN 'true' ELSE 'false' END),
		'diagnostic',json_extract(r.value,'$.diagnostic'),'submission_diagnostic',json_extract(r.value,'$.submission_diagnostic'),
		'run',CASE WHEN json_type(r.value,'$.run')='object' THEN json_object(
		'status',json_extract(r.value,'$.run.status'),'stop_status',json_extract(r.value,'$.run.stop_status'),
		'diagnostic',json_extract(r.value,'$.run.diagnostic'),'updated_at',json_extract(r.value,'$.run.updated_at'),
		'execution',json_object('attempt_id',json_extract(r.value,'$.run.execution.attempt_id'))) ELSE NULL END)), '[]') FROM json_each(a.payload,'$.rounds') r)))`
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
	case "task_candidate":
		table = "runner_candidates a"
		id = "a.candidate_id"
		payload = "json_remove(CAST(a.payload AS TEXT),'$.contract','$.submit','$.test_plan','$.runner_build','$.environment','$.actions','$.projection.final')"
		payload = `CASE WHEN json_type(a.payload,'$.projection')='object' THEN json_set(json_remove(` + payload + `,'$.projection.question'),'$.projection.question',CASE WHEN json_type(a.payload,'$.projection.question')='object' THEN json('{}') ELSE NULL END) ELSE ` + payload + ` END`
		created = "json_extract(CAST(a.payload AS TEXT),'$.created_at')"
		finished = "json_extract(CAST(a.payload AS TEXT),'$.projection.updated_at')"
		related = `json_object('task_status',(SELECT t.status FROM tasks t WHERE t.project_identity=a.project_identity AND t.task_id=a.task_id),'title',(SELECT json_extract(CAST(t.payload AS TEXT),'$.title') FROM tasks t WHERE t.project_identity=a.project_identity AND t.task_id=a.task_id),'winner',(SELECT d.selected_workflow_run_id FROM task_deliveries d WHERE d.project_identity=a.project_identity AND d.task_id=a.task_id),'progress',json((SELECT CAST(p.payload AS TEXT) FROM candidate_progress p WHERE p.project_identity=a.project_identity AND p.candidate_id=a.candidate_id)))`
		request = "a.candidate_id IN (%s)"
	case "roadmap":
		table = `(SELECT *,row_number() OVER (PARTITION BY project_identity,json_extract(payload,'$.source.id') ORDER BY json_extract(payload,'$.created_at') DESC,roadmap_id DESC) AS source_rank FROM roadmaps) a`
		id = "a.roadmap_id"
		payload = `json_object('project',a.project_identity,'id',a.roadmap_id,'revision',a.revision,
		'purpose',json_extract(a.payload,'$.purpose'),'state',json_extract(a.payload,'$.state'),
		'created_at',json_extract(a.payload,'$.created_at'),'request_id',json_extract(a.payload,'$.request_id'),'run_id',json_extract(a.payload,'$.run_id'),
		'diagnostic',json_extract(a.payload,'$.diagnostic'),'submission',json_extract(a.payload,'$.submission'),
		'cancelled',json(CASE WHEN json_extract(a.payload,'$.cancelled') THEN 'true' ELSE 'false' END),'source',json_object('path',json_extract(a.payload,'$.source.path')),
		 'publication',json_extract(a.payload,'$.publication'),
        'generations',json((SELECT COALESCE(json_group_array(json_object(
          'generation_id',json_extract(g.value,'$.generation_id'),'local_key',json_extract(g.value,'$.local_key'),
          'created_at',json_extract(g.value,'$.created_at'),'state',json_extract(g.value,'$.state'),
          'submission',json_extract(g.value,'$.submission'),'run_id',json_extract(g.value,'$.run_id'),
          'diagnostic',json_extract(g.value,'$.diagnostic'),
          'projection',CASE WHEN json_type(g.value,'$.projection')='object' THEN json_object('status',json_extract(g.value,'$.projection.status'),'stop_status',json_extract(g.value,'$.projection.stop_status'),'resume_required',json(CASE WHEN json_extract(g.value,'$.projection.resume_required') THEN 'true' ELSE 'false' END),'updated_at',json_extract(g.value,'$.projection.updated_at')) ELSE NULL END)), '[]') FROM json_each(a.payload,'$.generations') g)),
		'operation',json((SELECT json_object('id',c.command_id,'action',json_extract(c.payload,'$.action'),'status',c.status,'instance',json_extract(c.payload,'$.instance_id')) FROM commands c WHERE c.project_identity=a.project_identity AND c.kind='roadmap_command' AND c.target_id=a.roadmap_id AND json_extract(c.payload,'$.action') IN ('roadmap_publish','roadmap_admit','roadmap_resume') ORDER BY c.created_at DESC,c.command_id DESC LIMIT 1)),
		'projection',CASE WHEN json_type(a.payload,'$.projection')='object' THEN json_object(
		'status',json_extract(a.payload,'$.projection.status'),'stop_status',json_extract(a.payload,'$.projection.stop_status'),'execution',json_object('attempt_id',json_extract(a.payload,'$.projection.execution.attempt_id')),
		'resume_required',json(CASE WHEN json_extract(a.payload,'$.projection.resume_required') THEN 'true' ELSE 'false' END),'updated_at',json_extract(a.payload,'$.projection.updated_at')) ELSE NULL END)`
		created = "json_extract(a.payload,'$.created_at')"
		finished = "CASE WHEN json_type(a.payload,'$.publication') IS NULL THEN json_extract(a.payload,'$.projection.updated_at') ELSE NULL END"
		related = `json_object('current',a.source_rank=1,
		'requests',json((SELECT COALESCE(json_group_array(r.request_id),'[]') FROM roadmap_requests r WHERE r.project_identity=a.project_identity AND r.roadmap_id=a.roadmap_id)))`
		request = `EXISTS(SELECT 1 FROM roadmap_requests r WHERE r.project_identity=a.project_identity AND r.roadmap_id=a.roadmap_id AND r.request_id IN (%s))`
	default:
		return nil, false, fmt.Errorf("unknown activity source %q", in.Source)
	}
	related = "json_set(" + related + ",'$.observation',json((SELECT json_object('observed_at',NULLIF(o.last_success,''),'diagnostic',o.diagnostic) FROM source_observations o WHERE o.project_identity=a.project_identity AND o.source='" + in.Source + "' AND o.source_id=" + id + ")))"
	if in.Source == "task_candidate" {
		related = `json_set(` + related + `,'$.pending_effects',json(CASE WHEN EXISTS(SELECT 1 FROM json_each(a.payload,'$.actions') action WHERE json_extract(action.value,'$.state')='unknown' OR json_extract(action.value,'$.state')='accepted' AND COALESCE(json_extract(action.value,'$.action_status'),'') NOT IN ('completed','failed','rejected','cancelled')) THEN 'true' ELSE 'false' END))`
	}
	if in.Filter != "summary" && len(in.RequestIDs) == 0 {
		message := readingPreviewSQL(in.Source, id)
		related = "json_set(" + related + ",'$.message',json(" + message + "))"
	}
	args := []any{project}
	where := "a.project_identity=?"
	if in.SourceID != "" {
		where += " AND " + id + "=?"
		args = append(args, in.SourceID)
	}
	if in.Source == "baseline_check" {
		where += " AND a.kind='project_maintenance' AND json_extract(CAST(a.payload AS TEXT),'$.action')='maintenance_check'"
	}

	if len(in.RequestIDs) == 0 && in.Filter != "summary" && in.SourceID == "" {
		terminal := "COALESCE(json_extract(CAST(a.payload AS TEXT),'$.state'),'') IN ('ready','failed','cancelled')"
		switch in.Source {
		case "main_agent_advisory":
			terminal = "(json_extract(a.payload,'$.settled')=1 AND " + terminal + ")"
			if in.Filter == "recent" || in.Filter == "history" {
				where += " AND " + terminal
			} else if in.Filter == "ongoing" {
				where += " AND (NOT " + terminal + " OR json_extract(a.payload,'$.source_active')=1)"
			}
		case "environment_detection":
			unsettled := "EXISTS(SELECT 1 FROM json_each(CAST(a.payload AS TEXT),'$.rounds') r WHERE COALESCE(json_extract(r.value,'$.settled'),0)=0)"
			if in.Filter == "recent" || in.Filter == "history" {
				where += " AND (" + terminal + ")"
			} else if in.Filter == "ongoing" {
				where += " AND (NOT (" + terminal + ") OR " + unsettled + ")"
			}
		case "baseline_check":
			if in.Filter == "recent" || in.Filter == "history" {
				where += " AND a.status IN ('applied','rejected')"
			} else if in.Filter == "ongoing" {
				where += " AND a.status NOT IN ('applied','rejected')"
			}
		case "task_candidate":
			if in.Filter == "recent" || in.Filter == "history" {
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
	if in.Filter == "history" {
		created = "NULLIF(" + created + ",'0001-01-01T00:00:00Z')"
		finished = "NULLIF(" + finished + ",'0001-01-01T00:00:00Z')"
	}
	desc := in.Filter == "recent" || in.Filter == "history"
	if desc {
		sortAt = "COALESCE(" + finished + ",'')"
	} else {
		sortAt = "COALESCE(" + created + ",'')"
	}
	if in.Filter == "history" {
		sortAt = "CASE WHEN COALESCE(" + finished + ",'')<>'' THEN '2|'||" + finished + " WHEN COALESCE(" + created + ",'')<>'' THEN '1|'||" + created + " ELSE '0|' END"
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
			relation["blocked"] = record.Blocked()
		}
		if in.Source == "roadmap" {
			var record roadmap.Stored
			if err = json.Unmarshal(f.Payload, &record); err != nil {
				return nil, false, err
			}
			relation["settled"] = record.Value().Settled()
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
