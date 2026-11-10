import { NavLink } from "react-router-dom";
import { useState } from "react";
import { Api } from "../../shared/api/client";
import { short } from "../../shared/ui/status";
import { DeveloperSettings } from "./developer-settings";
import { BillingSettings } from "./billing-settings";
import { MainAgentSettings } from "./main-agent-settings";
import { UsageSettings } from "./usage-settings";
import { NavigationGuard } from "../../shared/ui/navigation-guard";
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
  const [generationDirty, setGenerationDirty] = useState(false);
  const [developerDirty, setDeveloperDirty] = useState(false);
  const [billingDirty, setBillingDirty] = useState(false);
  const [usageDirty, setUsageDirty] = useState(false);
  const [mainAgentDirty, setMainAgentDirty] = useState(false);
  return (
    <>
      <NavigationGuard
        dirty={
          generationDirty ||
          developerDirty ||
          billingDirty ||
          usageDirty ||
          mainAgentDirty
        }
      />
      <div className="page-heading">
        <div>
          <div className="eyebrow">SETTINGS</div>
          <h1>配置</h1>
          <p>管理全局配置，准备和验收项目运行环境。</p>
        </div>
      </div>
      <div className="configuration-panels">
        <MainAgentSettings {...settings} onDirtyChange={setMainAgentDirty} />
        <GenerationSettings {...settings} onDirtyChange={setGenerationDirty} />
        <DeveloperSettings {...settings} onDirtyChange={setDeveloperDirty} />
        <BillingSettings {...settings} onDirtyChange={setBillingDirty} />
        <UsageSettings {...settings} onDirtyChange={setUsageDirty} />
        <section className="settings-panel" aria-label="项目运行环境">
          <h2>项目运行环境</h2>
          <p className="muted">
            按执行环境准备项目依赖、运行验收，查看就绪状态与历史报告。
          </p>
          {activeProject ? (
            <NavLink to={`/projects/${activeProject}/environment`}>
              打开项目运行环境 →
            </NavLink>
          ) : (
            <p className="muted">选择项目后可检查项目运行环境。</p>
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
