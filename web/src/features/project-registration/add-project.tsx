import { useState, type FormEvent } from "react";
import { NavLink } from "react-router-dom";
import { ApiError, json } from "../../shared/api/client";
import type { ConfiguredProject } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";

const initial = {
  name: "",
  repository: "",
  remote: "",
  branch: "",
  provider: "gitea",
  base_url: "",
  pr_repository: "",
  token_env: "",
};

interface Registration {
  repository: string;
  commit: string;
  config_id: string;
  project_id: string;
  confirmation?: string;
  saved: boolean;
}

const fields = [
  ["name", "项目名称", "例如 my-project", "[A-Za-z0-9][A-Za-z0-9._\\-]{0,63}"],
  ["repository", "服务机器上的仓库路径", "例如 C:/Projects/my-project"],
  ["remote", "Git remote", "例如 origin"],
  ["branch", "目标分支", "例如 main"],
  ["base_url", "PR 服务地址", "例如 https://git.example.com"],
  ["pr_repository", "PR 仓库", "owner/repository"],
] as const;

export function AddProject({
  csrf,
  enabled,
  projects,
}: {
  csrf: string;
  enabled: boolean;
  projects: ConfiguredProject[];
}) {
  const [draft, setDraft] = useState(initial);
  const [result, setResult] = useState<Registration>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  function change(field: keyof typeof initial, value: string) {
    setDraft((current) => ({ ...current, [field]: value }));
    setResult(undefined);
    setError("");
  }

  async function request(confirmation?: string) {
    if (busy || !enabled) return;
    setError("");
    setBusy(true);
    try {
      const checked = await json<Registration>(
        "/api/v2/application/projects/registration",
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": csrf,
          },
          body: JSON.stringify({
            ...Object.fromEntries(
              Object.entries(draft).map(([key, value]) => [key, value.trim()]),
            ),
            token_env: draft.provider === "gitea" ? draft.token_env.trim() : "",
            confirmation,
          }),
        },
      );
      setResult(checked);
    } catch (failure) {
      if (
        !confirmation ||
        (failure instanceof ApiError &&
          failure.status >= 400 &&
          failure.status < 500)
      ) {
        setResult(undefined);
      }
      setError(failure instanceof Error ? failure.message : "项目操作失败");
    } finally {
      setBusy(false);
    }
  }

  function verify(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void request();
  }

  const loaded = projects.find(
    (project) => project.project_id === result?.project_id && project.online,
  );
  return (
    <section
      className="settings-panel project-registration"
      aria-labelledby="add-project-title"
    >
      <h2 id="add-project-title">添加已初始化项目</h2>
      <p className="muted">
        先按所采用版本的
        <a
          href="https://github.com/erranliu/SteerLine/blob/HEAD/SteerlineProjectGuideDoc/README.md"
          target="_blank"
          rel="noreferrer"
        >
          项目接入指南
        </a>
        准备仓库，并将初始化材料合入目标分支。
      </p>
      {!enabled && <p role="alert">会话已失效，请重新打开 SteerLine Web。</p>}
      {!result?.saved && (
        <form onSubmit={verify} aria-busy={busy}>
          <fieldset disabled={busy || !enabled}>
            {fields.map(([field, label, placeholder, pattern]) => (
              <label key={field}>
                {label}
                <input
                  required
                  pattern={pattern}
                  placeholder={placeholder}
                  value={draft[field]}
                  onChange={(event) => change(field, event.target.value)}
                />
              </label>
            ))}
            <label>
              PR 提供方
              <select
                value={draft.provider}
                onChange={(event) => change("provider", event.target.value)}
              >
                <option value="gitea">Gitea</option>
                <option value="github">GitHub</option>
              </select>
            </label>
            {draft.provider === "gitea" ? (
              <label>
                Gitea 令牌环境变量名（可选）
                <input
                  placeholder="例如 GITEA_TOKEN"
                  value={draft.token_env}
                  onChange={(event) => change("token_env", event.target.value)}
                />
              </label>
            ) : (
              <p className="muted">GitHub 使用服务机器上已认证的 gh。</p>
            )}
          </fieldset>
          <div className="project-registration-actions">
            <Button
              type="submit"
              variant={result?.confirmation ? "outline" : "default"}
              disabled={busy || !enabled}
            >
              核验目标分支
            </Button>
            {result?.confirmation && (
              <Button
                type="button"
                variant="default"
                disabled={busy || !enabled}
                onClick={() => void request(result.confirmation)}
              >
                确认添加
              </Button>
            )}
          </div>
        </form>
      )}
      {busy && (
        <p role="status">
          正在{result?.confirmation ? "处理项目请求" : "核验目标分支"}…
        </p>
      )}
      {error && <p role="alert">{error}</p>}
      {result && (
        <div role="status">
          <p>
            {result.saved
              ? loaded
                ? "项目已加载"
                : "已保存，待重启生效"
              : "初始化材料核验通过"}
          </p>
          <dl>
            <dt>仓库</dt>
            <dd>{result.repository}</dd>
            <dt>目标提交</dt>
            <dd>
              <code>{result.commit}</code>
            </dd>
            <dt>安装实例配置</dt>
            <dd>
              <code>etc/{result.config_id}</code>
            </dd>
          </dl>
          {result.saved &&
            (loaded ? (
              <NavLink to={`/projects/${loaded.project_id}/dag`}>
                打开项目
              </NavLink>
            ) : (
              <p>
                请在当前安装实例运行 <code>steerline restart</code> 或{" "}
                <code>restart.bat</code>，然后从项目切换器打开项目。
              </p>
            ))}
        </div>
      )}
    </section>
  );
}
