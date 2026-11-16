import {
  useQueries,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { abandonedActivitiesResource } from "../../shared/api/queries";
import { withActivityAbandonmentMark, type Activity } from "./model";

const marksKey = (scope: string[]) => [...scope, "abandoned-activity-marks"];
const activityKey = (source: string, id: string) => source + "/" + id;

export function rememberAbandonedActivities(
  client: QueryClient,
  scope: string[],
  activities: Pick<Activity, "source" | "source_id" | "abandoned_at">[],
) {
  const marks = Object.fromEntries(
    activities.flatMap((a) =>
      a.abandoned_at
        ? [[activityKey(a.source, a.source_id), a.abandoned_at]]
        : [],
    ),
  );
  if (Object.keys(marks).length) {
    client.setQueryData<Record<string, string>>(marksKey(scope), (old) => ({
      ...old,
      ...marks,
    }));
  }
}

export function useAbandonedActivities(source = "", ids: string[] = []) {
  const { api, identity, online } = useWorkspace();
  const client = useQueryClient();
  const scope = [api.home, identity.project_id, identity.instance_id];
  const localKey = marksKey(scope);
  const unique = [...new Set(ids.filter(Boolean))].sort();
  const batches = Array.from(
    { length: Math.ceil(unique.length / 100) },
    (_, i) => unique.slice(i * 100, i * 100 + 100),
  );
  const [local, ...remote] = useQueries({
    queries: [
      {
        queryKey: localKey,
        enabled: false,
        queryFn: async () => ({}) as Record<string, string>,
      },
      ...batches.map((batch) => ({
        queryKey: [...scope, abandonedActivitiesResource, source, batch],
        queryFn: async ({ signal }: { signal: AbortSignal }) => {
          const params = new URLSearchParams({
            mode: "abandoned",
            source,
            source_ids: batch.join(","),
          });
          const marks = await api.get<Record<string, string>>(
            "/activities?" + params,
            signal,
          );
          if (marks && !Array.isArray(marks) && typeof marks === "object") {
            const found = Object.fromEntries(
              batch.flatMap((id) =>
                typeof marks[id] === "string"
                  ? [[activityKey(source, id), marks[id]]]
                  : [],
              ),
            );
            if (Object.keys(found).length)
              client.setQueryData<Record<string, string>>(localKey, (old) => ({
                ...old,
                ...found,
              }));
          }
          return marks;
        },
        enabled: online,
        retry: false,
        staleTime: 5000,
      })),
    ],
  });
  return {
    isAbandoned: (id?: string) =>
      !!id && !!local.data?.[activityKey(source, id)],
    applyMark: (activity: Activity) =>
      withActivityAbandonmentMark(
        activity,
        local.data?.[activityKey(activity.source, activity.source_id)],
      ),
    pending: online && remote.some((query) => query.isPending),
  };
}
