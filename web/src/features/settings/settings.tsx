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
          <p>管理生成并发，准备和验收项目运行环境。</p>
        </div>
      </div>
      <div className="configuration-panels">
        <GenerationSettings {...settings} />
        <section className="settings-panel" aria-label="运行环境">
          <h2>运行环境</h2>
          <p className="muted">
            按执行环境准备项目依赖、运行验收，查看就绪状态与历史报告。
          </p>
          {activeProject ? (
            <NavLink to={`/projects/${activeProject}/environment`}>
              打开运行环境 →
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
