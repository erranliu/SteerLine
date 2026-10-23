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
  level: string;
  environment: string;
  status: string;
  note?: string;
  commit: string;
  path: string;
  at: string;
};
export type RecordPage = {
  records: RecordRow[];
  cursor: string;
  commit: string;
  path: string;
  observed_at: string;
  summary: {
    total: number;
    failed: number;
    latest?: RecordRow;
    last_passed?: RecordRow;
  };
};
