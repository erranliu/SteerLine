import { useEffect, useSyncExternalStore } from "react";
import { useQueryClient, type QueryKey } from "@tanstack/react-query";

function subscribe(changed: () => void) {
  document.addEventListener("visibilitychange", changed);
  return () => document.removeEventListener("visibilitychange", changed);
}

export function usePageVisible() {
  return useSyncExternalStore(
    subscribe,
    () => document.visibilityState !== "hidden",
  );
}

// Gate a selected query and cancel only its exact key on hiding or disposal.
// Poll intervals and completion rules remain with the feature.
export function useQueryVisibility(enabled: boolean, queryKey: QueryKey) {
  const visible = usePageVisible();
  const client = useQueryClient();
  const key = JSON.stringify(queryKey);
  const active = enabled && visible;
  useEffect(() => {
    if (!active)
      void client.cancelQueries({ queryKey: JSON.parse(key), exact: true });
  }, [active, client, key]);
  useEffect(
    () => () => {
      void client.cancelQueries({ queryKey: JSON.parse(key), exact: true });
    },
    [client, key],
  );
  return active;
}
