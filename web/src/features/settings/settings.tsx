import { NavLink } from "react-router-dom";
import { Api } from "../../shared/api/client";
import { short } from "../../shared/ui/status";
import {
  GenerationSettings,
  type GenerationSettingsProps,
} from "./generation-settings";

export function SettingsPage({
  api,
  online,
  projectId,
  ...settings
}: GenerationSettingsProps & {
  api?: Api;
  online?: boolean;
  projectId?: string;
}) {
  const activeProject = api?.identity.project_id ?? projectId;
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">SETTINGS</div>
          <h1>配置</h1>
          <p>管理生成并发，检查项目运行环境。</p>
        </div>
      </div>
      <div className="configuration-panels">
        <GenerationSettings {...settings} />
        <section className="settings-panel" aria-label="环境检查">
          <h2>环境检查</h2>
          <p className="muted">
            主动验证 Worker 的执行权限与 Git / PR
            能力，查看检查历史和待处理资源。
          </p>
          {activeProject ? (
            <NavLink to={`/projects/${activeProject}/environment`}>
              打开项目环境适配 →
            </NavLink>
          ) : (
            <p className="muted">选择项目后可检查其运行环境。</p>
          )}
        </section>
        {api && (
          <section className="settings-panel" aria-label="项目信息">
            <h2>项目信息</h2>
            <p className="path">{api.identity.repository_identity}</p>
            <p className="muted">
              项目 {short(api.identity.project_id)} · 服务实例{" "}
              {short(api.identity.instance_id)}
              {online ? "" : " · 连接待核实"}
            </p>
          </section>
        )}
      </div>
    </>
  );
}
