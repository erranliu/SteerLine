import { useQuery } from "@tanstack/react-query";
import { NavLink } from "react-router-dom";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import { Status, short } from "../../shared/ui/status";
export function SettingsPage({ api, online }: { api: Api; online: boolean }) {
  const result = useQuery({
    ...projectQueries(api).presentation(),
    enabled: online,
  });
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">PROJECT / SETTINGS</div>
          <h1>执行设置</h1>
          <p>当前服务已保存的执行方案与默认配置。</p>
        </div>
      </div>
      <section className="settings-panel">
        <h2>环境检查</h2>
        <p>
          主动验证 Worker 的执行权限与 Git / PR 能力，查看检查历史和待处理资源。
        </p>
        <NavLink
          to={"/projects/" + api.identity.project_id + "/environment-checks"}
        >
          打开环境检查 →
        </NavLink>
        <h2>已有执行方案</h2>
        {result.isPending && <p className="muted">正在读取执行方案…</p>}
        <p className="muted">
          方案由服务配置维护。每次启动可选择方案，冻结参数由 Go 服务核验。
        </p>
        {result.data?.presentation.profiles.map((p) => (
          <article className="profile" key={p.id}>
            <div>
              <strong>{p.name}</strong>
              {result.data.presentation.default_profile_id === p.id && (
                <Status value="ready" />
              )}
            </div>
            <p>
              {p.host} / {p.agent}
            </p>
            {p.details?.map((d, i) => (
              <p className="muted" key={i}>
                {d}
              </p>
            ))}
            {p.unavailable_reason && (
              <p className="notice amber">{p.unavailable_reason}</p>
            )}
          </article>
        ))}
        {result.error && <p className="notice amber">{String(result.error)}</p>}
        <h3>本机连接</h3>
        <p className="path">{api.identity.repository_identity}</p>
        <p className="muted">
          项目 {short(api.identity.project_id)} · 服务实例{" "}
          {short(api.identity.instance_id)}
        </p>
      </section>
    </>
  );
}
