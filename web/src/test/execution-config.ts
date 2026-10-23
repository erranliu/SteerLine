import type { ConfigSnapshot } from "../shared/api/execution-configs";

export const configSnapshot: ConfigSnapshot = {
  saved_digest: "loaded",
  writable: true,
  efforts: [],
  combinations: [],
  local_helper: { available: false },
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
