import { NavLink } from "react-router-dom";
import {
  Activity,
  Bot,
  FlaskConical,
  Layers,
  Monitor,
  SlidersHorizontal,
  Wallet,
} from "lucide-react";

const projectPages = [
  { path: "status", label: "项目状态", icon: Activity },
  { path: "environment", label: "项目运行环境", icon: Monitor },
  { path: "tests", label: "测试管理", icon: FlaskConical },
  { path: "settings", label: "任务自动派发", icon: Bot },
];

const globalPages = [
  { path: "/settings", label: "通用设置", icon: SlidersHorizontal },
  { path: "/settings/execution-configs", label: "执行方案管理", icon: Layers },
  { path: "/settings/billing", label: "计费", icon: Wallet },
];

export function configurationPage(pathname: string, projectId?: string) {
  const globalPage = globalPages.find(({ path }) => path === pathname);
  if (globalPage) return { section: "SteerLine 配置", title: globalPage.label };
  const projectPage =
    projectId &&
    projectPages.find(
      ({ path }) => pathname === `/projects/${projectId}/${path}`,
    );
  if (projectPage) return { section: "项目设置", title: projectPage.label };
}

export function ConfigurationNavigation({ projectId }: { projectId?: string }) {
  return (
    <nav className="configuration-nav" aria-label="配置导航">
      {projectId && (
        <div role="group" aria-labelledby="project-settings-nav">
          <h2 id="project-settings-nav">项目设置</h2>
          {projectPages.map(({ path, label, icon: Icon }) => (
            <NavLink key={path} end to={`/projects/${projectId}/${path}`}>
              <Icon size={17} aria-hidden="true" />
              <span>{label}</span>
            </NavLink>
          ))}
        </div>
      )}
      <div role="group" aria-labelledby="steerline-settings-nav">
        <h2 id="steerline-settings-nav">SteerLine 配置</h2>
        {globalPages.map(({ path, label, icon: Icon }) => (
          <NavLink key={path} end to={path}>
            <Icon size={17} aria-hidden="true" />
            <span>{label}</span>
          </NavLink>
        ))}
      </div>
    </nav>
  );
}
