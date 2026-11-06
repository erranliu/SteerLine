import type { ConfigSnapshot } from "../shared/api/execution-configs";

export const configSnapshot: ConfigSnapshot = {
  environments: [
    {
      adapter: "codex-app-server",
      target: { kind: "local" },
      profile_ids: ["local"],
    },
  ],
  saved_digest: "loaded",
  writable: true,
  efforts: [],
  combinations: [],
  profile_views: {
    local: {
      editable: true,
      view: {
        adapter: "codex-app-server",
        target: { kind: "local" },
        agent: "codex",
        model: "large",
        access_mode: "full_access",
      },
    },
  },
  workflows: [],
  config: {
    version: 3,
    runner: { executable: "runner", version: "1", sha256: "sha" },
    default_execution_profile: "local",
    execution_profiles: [
      {
        id: "local",
        name: "本机方案",
        execution: {
          adapter: "codex-app-server",
          target: { kind: "local" },
          settings: { model: "large" },
        },
        workflow: { id: "development", version: 2 },
      },
    ],
  },
};
