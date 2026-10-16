import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Api } from "../shared/api/client";
import { invalidateCommand, receiptChanged } from "../shared/api/queries";
import { listRequests } from "../shared/api/journal";
import type { StoredRequest } from "../shared/api/types";

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
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const pending = (await listRequests()).filter(
          (row) =>
            row.envelope.project_id === identity.project_id &&
            (!row.response ||
              ["submitted", "accepted", "outcome_unknown"].includes(
                row.response.status,
              )),
        );
        for (const row of pending) {
          if (controller.signal.aborted) return;
          try {
            const result = await api.reconcile(row.envelope, controller.signal);
            if (controller.signal.aborted) return;
            if (receiptChanged(row.response, result))
              void invalidateCommand(queries, api.home, row.envelope);
            else
              void queries.invalidateQueries({
                queryKey: [api.home, "local-requests"],
              });
          } catch {
            /* unknown stays in journal */
          }
        }
      } catch {
        // The journal query displays storage errors; keep this loop bounded.
      } finally {
        if (!controller.signal.aborted)
          timer = setTimeout(() => void poll(), 2000);
      }
    };
    void poll().catch(() => {});
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [online, api, queries]);
  const send = async (
    action: string,
    target: string,
    version: number,
    payload: Record<string, unknown>,
    extra: Record<string, number> = {},
  ) => {
    if (!online) throw new Error("当前连接不能变更业务状态");
    try {
      const result = await mutation.mutateAsync(
        api.envelope(action, target, version, payload, extra),
      );
      if (result.status === "rejected" || result.status === "outcome_unknown") {
        setError(result.diagnostic || "操作结果待核实，请查询原请求");
        setErrorRequest(result.request_id);
        return false;
      }
      return true;
    } catch {
      return false;
    }
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
    busy: mutation.isPending,
    reconcile: (row: StoredRequest) => reconcile.mutate(row),
    reconciling: reconcile.isPending,
  };
}
