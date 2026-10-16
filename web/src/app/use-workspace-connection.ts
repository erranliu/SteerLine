import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useSyncExternalStore,
} from "react";
import { onlineManager, useQuery, useQueryClient } from "@tanstack/react-query";
import { Api } from "../shared/api/client";
import { projectQueries, refreshProject } from "../shared/api/queries";
import type { Identity } from "../shared/api/types";

export function useWorkspaceConnection({
  initial,
  applicationConnected,
  home,
  connectionError,
  onRefreshConnection,
}: {
  initial: { csrf_token: string; identity: Identity };
  applicationConnected: boolean;
  home: string;
  connectionError?: unknown;
  onRefreshConnection?: () => void;
}) {
  const queries = useQueryClient();
  const networkOnline = useSyncExternalStore(
    (changed) => onlineManager.subscribe(changed),
    () => onlineManager.isOnline(),
  );
  const identity = initial.identity;
  const api = useMemo(
    () => new Api(identity, initial.csrf_token, home),
    [identity.project_id, identity.instance_id, initial.csrf_token, home],
  );
  useLayoutEffect(() => {
    api.activate();
    return () => {
      api.deactivate();
      void queries.cancelQueries({
        queryKey: [api.home, api.identity.project_id, api.identity.instance_id],
      });
    };
  }, [api, queries]);
  const connected = applicationConnected && identity.ready;
  const snapshot = useQuery({
    ...projectQueries(api).snapshot(),
    enabled: connected,
    refetchInterval: 2000,
    retry: false,
  });
  const page = snapshot.data?.connection.snapshot.page;
  const online =
    networkOnline &&
    connected &&
    !snapshot.isError &&
    snapshot.data?.connection.state === "connected" &&
    snapshot.data.connection.can_control &&
    snapshot.data.connection.identity_matched &&
    snapshot.data.connection.config_matched &&
    page?.service.instance_id === identity.instance_id;
  useEffect(() => {
    if (!online) return;
    const options = projectQueries(api);
    void queries.prefetchQuery(options.presentation());
    void queries.prefetchQuery(options.tasks());
  }, [online, api, queries]);
  const refresh = () => {
    onRefreshConnection?.();
    void refreshProject(queries, api);
  };
  const diagnostic = connectionError
    ? String(connectionError)
    : snapshot.error
      ? String(snapshot.error)
      : identity.diagnostic || snapshot.data?.connection.diagnostic;
  return { api, identity, snapshot, page, online, diagnostic, refresh };
}
