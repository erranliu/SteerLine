export type Definition = {
  id: string;
  name: string;
  level: string;
  environment: string;
  command: string;
  check: string;
  note?: string;
  current?: boolean;
};
export type Catalogue = {
  tests: Definition[];
  commit: string;
  path: string;
  observed_at: string;
  diagnostic?: string;
};
export type RecordRow = {
  id: string;
  test_id: string;
  name: string;
  level: string;
  environment: string;
  status: string;
  message: string;
  retest: string;
  retest_id?: string;
  retest_source_fixed?: boolean;
  started_at: string;
  finished_at: string;
  branch: string;
  commit: string;
  source_fixed: boolean;
  partial: boolean;
  rerun_of?: string;
  evidence_status: string;
  diagnostic?: string;
  source: {
    kind: string;
    task_id?: string;
    candidate_id?: string;
    run_id?: string;
  };
};
export type RecordPage = {
  records: RecordRow[];
  cursor: string;
  upper: number;
  observed_at: string;
  summary: {
    total: number;
    pending: number;
    latest?: RecordRow;
    last_passed?: RecordRow;
  };
};
export type RecordDetail = RecordRow & {
  report: {
    definition: Definition;
    command: string;
    repository: string;
    before_commit: string;
    after_commit: string;
    before_clean: boolean;
    after_clean: boolean;
    definitions: Record<string, string>;
    results: {
      test_id: string;
      status: string;
      message: string;
      log: string;
    }[];
    scope?: {
      complete: boolean;
      items: { id: string; status: string; message?: string }[];
    };
  };
  chain: RecordRow[];
};
export type RecordEvidence = {
  id: string;
  status: string;
  diagnostic: string;
  path: string;
  sha256: string;
  text: string;
};
