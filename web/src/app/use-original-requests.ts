import { useEffect, useRef, useState } from "react";
import {
  useMutation,
  useMutationState,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Api } from "../shared/api/client";
import { invalidateCommand, receiptChanged } from "../shared/api/queries";
import { listRequests, requestByID } from "../shared/api/journal";
import type { StoredRequest, Envelope } from "../shared/api/types";

export function useOriginalRequests(api: Api, online: boolean) {
  const queries = useQueryClient();
  const { identity, home } = api;
  const current = useRef(api);
  current.current = api;
  const [error, setError] = useState("");
  const [errorRequest, setErrorRequest] = useState("");
  const mutationKey = [
    home,
    identity.project_id,
    identity.instance_id,
    "original-command",
  ];
  const observations = useMutationState({
    filters: { mutationKey, exact: true },
    select: (item) => ({
      envelope: item.state.variables as Envelope,
      response: item.state.data as
        Awaited<ReturnType<Api["submit"]>> | undefined,
      status: item.state.status,
      at: item.state.submittedAt,
      error: item.state.error as (Error & { requestUnsent?: boolean }) | null,
    }),
  });
  // Read the actual mutation cache so a second activation before React renders
  // sees the first submission. This does not acquire locks or infer conflicts.
  const pendingRequests = () =>
    queries
      .getMutationCache()
      .findAll({ mutationKey, exact: true, status: "pending" })
      .map((item) => item.state.variables as Envelope);
  const mutation = useMutation({
    mutationKey,
    mutationFn: async (envelope: ReturnType<Api["envelope"]>) => {
      try {
        return await api.submit(envelope);
      } catch (error) {
        let requestUnsent = false;
        try {
          requestUnsent = !(await requestByID(envelope.request_id));
        } catch {
          /* Unknown is not unsent. */
        }
        throw Object.assign(
          error instanceof Error ? error : new Error(String(error)),
          { requestUnsent },
        );
      }
    },
    onMutate: (envelope) => {
      if (envelope.action === "environment_start") {
        setError("");
        setErrorRequest("");
      }
    },
    onError: async (e, envelope) => {
      if (current.current !== api) return;
      if (envelope.action.startsWith("refactoring_")) return;
      if (
        envelope.action === "environment_start" &&
        (await requestByID(envelope.request_id).catch(() => undefined))
      )
        return;
      if (current.current !== api) return;
      setError(String(e));
      setErrorRequest(envelope.request_id);
    },
    onSuccess: (_result, envelope) => {
      if (current.current !== api) return;
      if (envelope.action.startsWith("refactoring_")) return;
      setError("");
      setErrorRequest("");
    },
    onSettled: async (_result, _error, envelope) => {
      // Hand the saved original row to the existing journal query before the
      // mutation leaves pending. A failed read never erases its known receipt.
      const saved = await requestByID(envelope.request_id).catch(
        () => undefined,
      );
      if (saved)
        queries.setQueryData<StoredRequest[]>(
          [home, "local-requests"],
          (rows = []) => [
            ...rows.filter(
              (row) => row.envelope.request_id !== envelope.request_id,
            ),
            saved,
          ],
        );
      void invalidateCommand(queries, api.home, envelope);
    },
  });
  const requests = useQuery({
    queryKey: [home, "local-requests"],
    queryFn: listRequests,
    refetchInterval: 2000,
  });
  const observedRequests = new Map(
    (requests.data || []).map((row) => [row.envelope.request_id, row]),
  );
  for (const item of observations) {
    if (
      !item.envelope ||
      (item.status !== "pending" &&
        !item.response &&
        (!item.error || item.error.requestUnsent))
    )
      continue;
    const previous = observedRequests.get(item.envelope.request_id);
    if (
      previous &&
      ["applied", "rejected"].includes(previous.response?.status || "")
    )
      continue;
    observedRequests.set(item.envelope.request_id, {
      ...previous,
      envelope: item.envelope,
      created_at: previous?.created_at || new Date(item.at).toISOString(),
      response: item.response || previous?.response,
      error: previous?.error || (item.error ? String(item.error) : undefined),
    });
  }
  useEffect(() => {
    if (
      errorRequest &&
      requests.data?.find((row) => row.envelope.request_id === errorRequest)
        ?.response?.status === "applied"
    ) {
      setError("");
      setErrorRequest("");
    }
  }, [errorRequest, requests.dataUpdatedAt, requests.data]);
  useEffect(() => {
    if (!online) return;
    const controller = new AbortController();
    const inflight = new Set<string>();
    const next = new Map<string, { at: number; attempts: number }>();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const pending = (await listRequests()).filter(
          (row) =>
            row.envelope.project_id === identity.project_id &&
            !["applied", "rejected"].includes(row.response?.status ?? ""),
        );
        if (controller.signal.aborted) return;
        for (const row of pending) {
          const id = row.envelope.request_id;
          if (inflight.size >= 3) break;
          if (inflight.has(id) || (next.get(id)?.at ?? 0) > Date.now())
            continue;
          inflight.add(id);
          void api
            .reconcile(row.envelope, controller.signal)
            .then((result) => {
              if (
                !controller.signal.aborted &&
                receiptChanged(row.response, result)
              )
                void invalidateCommand(queries, api.home, row.envelope);
            })
            .catch(() => {
              // Api preserves the request and its own verification diagnostic.
            })
            .finally(() => {
              inflight.delete(id);
              if (controller.signal.aborted) return;
              const attempts = next.get(id)?.attempts ?? 0;
              next.set(id, {
                at:
                  Date.now() +
                  [2000, 5000, 10000, 30000][Math.min(attempts, 3)],
                attempts: attempts + 1,
              });
              void queries.invalidateQueries({
                queryKey: [api.home, "local-requests"],
              });
            });
        }
      } catch {
        // Storage errors are surfaced by the journal query.
      } finally {
        if (!controller.signal.aborted)
          timer = setTimeout(() => void poll(), 500);
      }
    };
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [online, api, queries]);
  const submit = async (envelope: Envelope) => {
    if (!online) throw new Error("当前连接不能变更业务状态");
    try {
      const result = await mutation.mutateAsync(envelope);
      if (current.current !== api) return result;
      if (
        !envelope.action.startsWith("refactoring_") &&
        (result.status === "rejected" ||
          (result.status === "outcome_unknown" &&
            result.code !== "request_receipt_persist_failed" &&
            !["environment_start", "start_task", "start_candidate"].includes(
              envelope.action,
            )))
      ) {
        setError(result.diagnostic || "操作结果待核实，请查询原请求");
        setErrorRequest(result.request_id);
      }
      return result;
    } catch {
      return undefined;
    }
  };
  const send = async (
    action: string,
    target: string,
    version: number,
    payload: Record<string, unknown>,
    extra: Record<string, number> = {},
  ) => {
    const result = await submit(
      api.envelope(action, target, version, payload, extra),
    );
    return (
      !!result &&
      result.status !== "rejected" &&
      result.status !== "outcome_unknown"
    );
  };
  const reconcile = useMutation({
    mutationFn: (row: StoredRequest) => api.reconcile(row.envelope),
    onSuccess: (result, row) => {
      if (receiptChanged(row.response, result))
        void invalidateCommand(queries, api.home, row.envelope);
    },
    onSettled: () =>
      queries.invalidateQueries({ queryKey: [api.home, "local-requests"] }),
    onError: (error, row) => {
      if (current.current !== api) return;
      if (
        row.envelope.action === "environment_start" ||
        row.envelope.action.startsWith("refactoring_") ||
        row.response?.code === "request_receipt_persist_failed" ||
        ["start_task", "start_candidate"].includes(row.envelope.action)
      )
        return;
      setError(String(error));
      setErrorRequest("");
    },
  });
  return {
    requests,
    observedRequests: [...observedRequests.values()],
    pendingRequests,
    error,
    clearError: () => setError(""),
    send,
    submit,
    busy: mutation.isPending,
    reconcile: (row: StoredRequest) => reconcile.mutate(row),
    reconciling: reconcile.isPending,
  };
}
