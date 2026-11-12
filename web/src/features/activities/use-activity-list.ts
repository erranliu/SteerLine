import { useEffect } from "react";
import {
  replaceEqualDeep,
  useInfiniteQuery,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import type { Api } from "../../shared/api/client";
import { usePageVisible } from "../../shared/query-visibility";
import type { ActivityPage, ActivityListFilter } from "./model";

// Both surfaces share single-page requests. Only the head has a timer; loaded
// pages are user-driven, including on focus/reconnect and in history.
export function useActivityList(
  api: Api,
  filter: ActivityListFilter,
  enabled: boolean,
  interval: number,
) {
  const client = useQueryClient();
  const scope = [api.home, api.identity.project_id, api.identity.instance_id];
  const key = [...scope, "async-activities", filter];
  const headKey = [...scope, "async-activities", "head", filter];
  const visible = usePageVisible();
  const active = enabled && visible;
  const read = async (cursor: string, signal?: AbortSignal) => {
    const params = new URLSearchParams({ filter, limit: "50" });
    if (cursor) params.set("cursor", cursor);
    const page = await api.get<ActivityPage>("/activities?" + params, signal);
    if (
      !Array.isArray(page.items) ||
      !Array.isArray(page.sources) ||
      !page.counts ||
      !page.request_links ||
      page.project_id !== api.identity.project_id ||
      page.instance_id !== api.identity.instance_id
    )
      throw new Error("活动范围已变化，等待重新读取");
    if (!cursor) {
      const prior = client.getQueryData<ActivityPage>(headKey);
      const failed = new Set(
        page.sources
          .filter((source) => source.diagnostic)
          .map((source) => source.source),
      );
      const ids = new Set(page.items.map((item) => item.activity_id));
      page.items = [
        ...page.items,
        ...(prior?.items.filter(
          (item) => failed.has(item.source) && !ids.has(item.activity_id),
        ) || []),
      ];
    }
    return page;
  };
  const headOptions = {
    queryKey: headKey,
    queryFn: ({ signal }: { signal: AbortSignal }) => read("", signal),
    staleTime: 1000,
    retry: false as const,
    refetchOnWindowFocus: filter === "history" ? false : ("always" as const),
    refetchOnReconnect: false,
    refetchOnMount: false,
  };
  const head = useQuery({
    ...headOptions,
    enabled: active,
    refetchInterval:
      active && filter !== "history" && interval > 0 ? interval : false,
  });
  const pages = useInfiniteQuery({
    queryKey: key,
    initialPageParam: "",
    queryFn: ({ signal, pageParam }) =>
      pageParam ? read(pageParam, signal) : client.fetchQuery(headOptions),
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
    enabled: false,
    retry: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const keyText = JSON.stringify(key);
  const headKeyText = JSON.stringify(headOptions.queryKey);
  useEffect(() => {
    const cancelUnused = () =>
      queueMicrotask(() => {
        for (const text of [keyText, headKeyText]) {
          const queryKey = JSON.parse(text);
          if (
            !client.getQueryCache().find({ queryKey, exact: true })?.isActive()
          )
            void client.cancelQueries({ queryKey, exact: true });
        }
      });
    if (!active) cancelUnused();
    return cancelUnused;
  }, [active, keyText, headKeyText, client]);
  useEffect(() => {
    if (!head.data) return;
    client.setQueryData([...scope, "activity-counts"], head.data);
    const listKey = JSON.parse(keyText);
    // Keep the cursor and its first page together during and after pagination.
    const fetchStatus = client.getQueryState(listKey)?.fetchStatus;
    if (fetchStatus && fetchStatus !== "idle") return;
    client.setQueryData<InfiniteData<ActivityPage, string>>(listKey, (old) =>
      old && (old.pages.length > 1 || old.pages[0] === head.data)
        ? undefined
        : { pages: [head.data!], pageParams: [""] },
    );
  }, [head.data, client, keyText, filter, pages.isFetchingNextPage]);
  const first = pages.data?.pages[0];
  const latest = head.data;
  const hasUpdates =
    (pages.data?.pages.length ?? 0) > 1 &&
    !!first &&
    !!latest &&
    !head.isError &&
    (first.next_cursor !== latest.next_cursor ||
      first.has_more !== latest.has_more ||
      replaceEqualDeep(first.items, latest.items) !== first.items);
  const refresh = async () => {
    const first = await client.fetchQuery({ ...headOptions, staleTime: 0 });
    await client.cancelQueries({ queryKey: key, exact: true });
    client.setQueryData(key, { pages: [first], pageParams: [""] });
    return first;
  };
  return {
    ...pages,
    error: head.error || pages.error,
    isError: head.isError || pages.isError,
    isFetching: head.isFetching || pages.isFetching,
    latest,
    hasUpdates,
    refreshObservation: () => head.refetch({ cancelRefetch: false }),
    refetch: refresh,
  };
}
