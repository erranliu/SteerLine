import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Api } from "../shared/api/client";
import { invalidateCommand, receiptChanged } from "../shared/api/queries";
import { listRequests } from "../shared/api/journal";
import type { StoredRequest, Envelope } from "../shared/api/types";

export function useOriginalRequests(api: Api, online: boolean) {
  const queries = useQueryClient();
  const { identity, home } = api;
  const [error, setError] = useState("");
  const [errorRequest, setErrorRequest] = useState("");
  const mutation = useMutation({
    mutationFn: (envelope: ReturnType<Api["envelope"]>) => api.submit(envelope),
    onError: (e, envelope) => {
      setError(String(e));
      setErrorRequest(envelope.request_id);
    },
    onSuccess: () => {
      setError("");
      setErrorRequest("");
    },
    onSettled: (_result, _error, envelope) => {
      void invalidateCommand(queries, api.home, envelope);
    },
  });
  const requests = useQuery({
    queryKey: [home, "local-requests"],
    queryFn: listRequests,
    refetchInterval: 2000,
  });
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
      if (result.status === "rejected" || result.status === "outcome_unknown") {
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
    onError: (error) => {
      setError(String(error));
      setErrorRequest("");
    },
  });
  return {
    requests,
    error,
    clearError: () => setError(""),
    send,
    submit,
    busy: mutation.isPending,
    reconcile: (row: StoredRequest) => reconcile.mutate(row),
    reconciling: reconcile.isPending,
  };
}
