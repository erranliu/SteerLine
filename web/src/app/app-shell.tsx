import { useState, type ReactNode } from "react";
import { NavLink, useLocation } from "react-router-dom";
import {
  ArrowLeft,
  ArrowUpRight,
  Command,
  FlaskConical,
  GitBranch,
  Layers,
  PanelLeftClose,
  Settings,
} from "lucide-react";
import { Button } from "../shared/ui/button";

export function AppShell({
  projectId,
  switcher,
  connection,
  children,
  footer,
  questions = 0,
  returnTo,
}: {
  projectId?: string;
  switcher: ReactNode;
  connection: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  questions?: number;
  returnTo?: string;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const location = useLocation();
  const global = location.pathname.startsWith("/settings");
  const title = location.pathname.endsWith("/execution-configs")
    ? "执行方案配置"
    : ({
        dag: "任务 DAG",
        runs: "执行与待处理",
        tests: "测试管理",
        settings: "配置",
        "environment-checks": "环境检查",
      }[location.pathname.split("/").pop() ?? ""] ?? "规划与执行");
  return (
    <div className={"app-shell " + (collapsed ? "sidebar-collapsed" : "")}>
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-symbol">◆</span>
          <strong>SteerLine</strong>
          <span className="brand-caption">WORKSPACE</span>
        </div>
        {switcher}
        <p className="nav-heading">项目</p>
        <nav aria-label="主导航">
          {projectId ? (
            <>
              <NavLink aria-label="任务 DAG" to={`/projects/${projectId}/dag`}>
                <GitBranch size={17} />
                <span>任务 DAG</span>
              </NavLink>
              <NavLink
                aria-label="执行与待处理"
                to={`/projects/${projectId}/runs`}
              >
                <Layers size={17} />
                <span>执行与待处理</span>
                {questions > 0 && (
                  <span className="nav-count">{questions}</span>
                )}
              </NavLink>
              <NavLink
                aria-label="测试管理"
                to={`/projects/${projectId}/tests`}
              >
                <FlaskConical size={17} />
                <span>测试管理</span>
              </NavLink>
            </>
          ) : (
            <NavLink to="/projects">
              <GitBranch size={17} />
              <span>选择项目</span>
            </NavLink>
          )}
        </nav>
        <div className="sidebar-footer">
          <NavLink
            aria-label="执行方案配置"
            title="执行方案配置"
            to="/settings/execution-configs"
          >
            <Settings size={16} />
            <span>执行方案配置</span>
          </NavLink>
          <NavLink
            aria-label="配置"
            title="配置"
            to={projectId ? `/projects/${projectId}/settings` : "/settings"}
          >
            <Settings size={16} />
            <span>配置</span>
          </NavLink>
          <div className="sidebar-note">
            <span className="connection-dot" /> LOCAL SERVICE{" "}
            <ArrowUpRight size={12} />
          </div>
        </div>
      </aside>
      <div className="main-shell">
        <header className="topbar">
          <div className="breadcrumb">
            <Button
              variant="ghost"
              size="sm"
              aria-label="切换导航宽度"
              onClick={() => setCollapsed(!collapsed)}
            >
              <PanelLeftClose size={17} />
            </Button>
            <span>{global ? "设置" : "当前项目"}</span>
            <span>/</span>
            <strong>{title}</strong>
          </div>
          <div className="topbar-actions">
            {returnTo && (
              <NavLink className="button button-ghost" to={returnTo}>
                <ArrowLeft size={14} />
                {returnTo === "/projects" ? "选择项目" : "返回项目"}
              </NavLink>
            )}
            {connection}
          </div>
        </header>
        <main>{children}</main>
        <footer className="app-footer">
          <span>
            <Command size={12} /> SteerLine · 本机 Web
          </span>
          <span>{footer}</span>
        </footer>
      </div>
    </div>
  );
}
