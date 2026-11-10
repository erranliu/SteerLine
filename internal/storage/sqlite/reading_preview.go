package sqlite

// Expressions below are selected only by the static source switch. No caller
// supplies a SQL expression, body path, origin table or projection predicate.
func readingPreviewSQL(source, id string) string {
	predicate := `f.role='assistant'`
	if source == "main_agent_advisory" {
		predicate = `f.role='assistant' AND f.kind='suggestion'`
	}
	if source == "task_candidate" {
		predicate += ` AND f.run_id=a.run_id AND f.attempt_id=json_extract(a.payload,'$.projection.execution.attempt_id') AND f.session_id=json_extract(a.payload,'$.projection.execution.session_id')`
	}
	archived := `(SELECT json_object('kind',f.kind,'text',f.preview,'at',NULLIF(f.at,''),'truncated',json(CASE WHEN f.body_bytes>length(CAST(f.preview AS BLOB)) THEN 'true' ELSE 'false' END)) FROM reading_fields f WHERE f.project_identity=a.project_identity AND f.source='` + source + `' AND f.source_id=` + id + ` AND ` + predicate + ` ORDER BY f.seq DESC LIMIT 1)`
	if source != "task_candidate" {
		return archived
	}
	message := `SELECT json_object('kind','assistant','text',m.text,'observed_at',NULLIF(m.observed_at,''),'truncated',json('false'),'diagnostic',m.diagnostic) FROM message_observations m WHERE m.project_identity=a.project_identity AND m.candidate_id=a.candidate_id AND m.run_id=a.run_id AND m.attempt_id=json_extract(a.payload,'$.projection.execution.attempt_id') AND m.session_id=json_extract(a.payload,'$.projection.execution.session_id')`
	live := `(` + message + ` AND m.text<>'')`
	question := `COALESCE(NULLIF(json_extract(a.payload,'$.projection.question.text'),''),(SELECT group_concat(json_extract(q.value,'$.text'),char(10)) FROM json_each(a.payload,'$.projection.question.native_questions') q))`
	current := `CASE WHEN json_type(a.payload,'$.projection.question')='object' THEN json_object('kind','current_question','text',substr(` + question + `,1,512),'truncated',json(CASE WHEN length(` + question + `)>512 THEN 'true' ELSE 'false' END)) END`
	return `COALESCE(` + current + `,CASE WHEN json_extract(a.payload,'$.projection.status') NOT IN ('completed','closed','failed','stopped') THEN ` + live + ` END,` + archived + `,` + live + `,(` + message + `))`
}
