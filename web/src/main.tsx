import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { authorize } from "./shared/api/client";
import { App } from "./app/app";
import "./styles.css";
const root = createRoot(document.getElementById("root")!);
const queries = new QueryClient({
  defaultOptions: {
    queries: { retry: false, refetchOnWindowFocus: false, staleTime: 1000 },
    mutations: { retry: false },
  },
});
root.render(
  <div className="opening">
    <span className="brand-symbol">◆</span>
    <h1>SteerLine</h1>
    <p>正在核验本机工作空间…</p>
  </div>,
);
authorize()
  .then((initial) =>
    root.render(
      <StrictMode>
        <QueryClientProvider client={queries}>
          <BrowserRouter>
            <App initial={initial} />
          </BrowserRouter>
        </QueryClientProvider>
      </StrictMode>,
    ),
  )
  .catch((error) =>
    root.render(
      <div className="opening">
        <span className="brand-symbol">◆</span>
        <h1>打开你的工作空间</h1>
        <p>请使用本机入口授权当前浏览器。</p>
        <code>steerline web -config &lt;项目配置&gt;</code>
        <p className="muted">{String(error)}</p>
        <button
          className="button button-outline"
          onClick={() => location.reload()}
        >
          重新核验
        </button>
      </div>,
    ),
  );
