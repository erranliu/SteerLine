import { createContext, useContext } from "react";
import type { Api } from "../shared/api/client";
import type {
  Snapshot,
  Identity,
  Envelope,
  Receipt,
  StoredRequest,
} from "../shared/api/types";
export interface Workspace {
  api: Api;
  identity: Identity;
  online: boolean;
  snapshot?: Snapshot;
  requests?: StoredRequest[];
  submit: (envelope: Envelope) => Promise<Receipt | undefined>;
  send: (
    action: string,
    target: string,
    version: number,
    payload: Record<string, unknown>,
    extra?: Record<string, number>,
  ) => Promise<boolean>;
  busy: boolean;
  refresh: () => void;
}
export const WorkspaceContext = createContext<Workspace | null>(null);
export function useWorkspace() {
  const value = useContext(WorkspaceContext);
  if (!value) throw new Error("Workspace is missing");
  return value;
}
