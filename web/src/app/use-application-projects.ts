import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";
import { json } from "../shared/api/client";
import type {
  BrowserSession,
  ConfiguredProject,
  Identity,
} from "../shared/api/types";
function readProjectPreference(
  storage: "sessionStorage" | "localStorage",
  key: string,
) {
  try {
    return window[storage].getItem(key);
  } catch {
    return null;
  }
}
function rememberProject(home: string, project: string) {
  for (const [storage, prefix] of [
    ["sessionStorage", "steerline-project:"],
    ["localStorage", "steerline-recent:"],
  ] as const) {
    try {
      window[storage].setItem(prefix + home, project);
    } catch {
      // Optional preferences cannot prevent URL-based project selection.
    }
  }
}

const projectRoute =
  /^\/projects\/([^/]+)\/(roadmaps|dag|runs|tests|settings|environment)$/;

function projectReturnTarget(
  home: string,
  projects: ConfiguredProject[],
  lastPath?: string,
) {
  const available = (id: string) =>
    projects.some((p) => p.project_id === id && !p.conflict);
  const saved =
    lastPath ??
    readProjectPreference("sessionStorage", "steerline-return:" + home);
  if (saved) {
    const path = saved.split(/[?#]/)[0].match(projectRoute);
    if (path && available(path[1])) return saved;
  }
  for (const candidate of [
    readProjectPreference("sessionStorage", "steerline-project:" + home),
    readProjectPreference("localStorage", "steerline-recent:" + home),
  ]) {
    if (candidate && available(candidate)) return `/projects/${candidate}/dag`;
  }
  return "/projects";
}

export function useApplicationProjects(initial: BrowserSession) {
  const session = useQuery({
    queryKey: ["browser-session", initial.identity.home_id],
    queryFn: () => json<BrowserSession>("/auth/status"),
    initialData: initial,
    refetchInterval: 2500,
    retry: false,
  });
  const home = initial.identity.home_id;
  const sessionValid =
    !session.isError && session.data.identity.home_id === home;
  const catalog = useQuery({
    queryKey: [home, "projects"],
    queryFn: ({ signal }) =>
      json<{ projects: ConfiguredProject[]; loaded_execution_digest?: string }>(
        "/api/v2/application/projects",
        {
          signal,
        },
      ),
    enabled: sessionValid,
    refetchInterval: 5000,
    retry: false,
  });
  const location = useLocation();
  const navigate = useNavigate();
  const lastProjectPath = useRef<{ home: string; path: string } | undefined>(
    undefined,
  );
  const projects = catalog.data?.projects ?? [];
  const match = location.pathname.match(projectRoute);
  const returnTo = projectReturnTarget(
    home,
    projects,
    lastProjectPath.current?.home === home
      ? lastProjectPath.current.path
      : undefined,
  );
  const global =
    location.pathname === "/settings" ||
    location.pathname.startsWith("/settings/");
  const id = match?.[1] ?? (global ? (returnTo.split("/")[2] ?? "") : "");
  const page = match?.[2] ?? "dag";
  const selected = projects.find((p) => p.project_id === id && !p.conflict);
  const projectIdentity = useQuery({
    queryKey: [home, id, "identity"],
    queryFn: ({ signal }) =>
      json<Identity>("/api/v2/application/projects/" + id + "/identity", {
        signal,
      }),
    enabled: sessionValid && !!selected,
    refetchInterval: 10000,
    retry: false,
  });
  const choose = (project: string) => {
    navigate("/projects/" + project + "/" + page);
  };
  const chosenOnce = useRef(false);
  useEffect(() => {
    if (global) return;
    if (id || chosenOnce.current || !catalog.data) return;
    const preferences = [
      readProjectPreference("sessionStorage", "steerline-project:" + home),
      readProjectPreference("localStorage", "steerline-recent:" + home),
    ];
    const recent = preferences.find(
      (candidate) =>
        candidate &&
        projects.some((p) => p.project_id === candidate && !p.conflict),
    );
    if (!preferences.some(Boolean)) chosenOnce.current = true;
    if (recent) {
      chosenOnce.current = true;
      navigate("/projects/" + recent + "/dag", { replace: true });
    }
  }, [id, catalog.data, home, navigate, projects, location.pathname]);
  useEffect(() => {
    if (selected && id) {
      rememberProject(home, id);
      if (match) {
        lastProjectPath.current = {
          home,
          path: location.pathname + location.search + location.hash,
        };
        try {
          sessionStorage.setItem(
            "steerline-return:" + home,
            lastProjectPath.current.path,
          );
        } catch {
          // Optional navigation preferences cannot prevent opening settings.
        }
      }
    }
  }, [
    selected?.project_id,
    home,
    id,
    location.pathname,
    location.search,
    location.hash,
  ]);
  return {
    session,
    home,
    sessionValid,
    catalog,
    id,
    projects,
    selected,
    projectIdentity,
    choose,
    returnTo,
  };
}
