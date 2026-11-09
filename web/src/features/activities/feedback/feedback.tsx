import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import "./feedback.css";

type Feedback = {
  entry?: RefObject<HTMLButtonElement | null>;
  notify: (title: string, origin?: HTMLElement | null) => void;
  open: () => void;
};
const Context = createContext<Feedback>({ notify: () => {}, open: () => {} });
export const useActivityFeedback = () => useContext(Context);

// This host has no request identity, receipt, lifecycle, or execution callback.
export function ActivityFeedbackProvider({
  children,
}: {
  children: ReactNode;
}) {
  const entry = useRef<HTMLButtonElement>(null);
  const active = useRef(false);
  const [visual, setVisual] = useState<{
    title: string;
    target: DOMRect;
    from?: DOMRect;
  }>();
  useEffect(() => {
    if (!visual) return;
    const clear = () => {
      active.current = false;
      setVisual(undefined);
    };
    const timer = window.setTimeout(clear, 400);
    window.addEventListener("resize", clear);
    window.addEventListener("scroll", clear, true);
    document.addEventListener("visibilitychange", clear);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("resize", clear);
      window.removeEventListener("scroll", clear, true);
      document.removeEventListener("visibilitychange", clear);
    };
  }, [visual]);
  const open = () => {
    if (entry.current?.getAttribute("aria-expanded") === "false")
      entry.current.click();
  };
  const notify: Feedback["notify"] = (title, origin) => {
    // Decoration may never prevent or fail the caller's original submission.
    try {
      if (active.current || document.visibilityState === "hidden") return;
      const target = entry.current?.getBoundingClientRect();
      if (
        !target?.width ||
        !target.height ||
        target.top < 0 ||
        target.left < 0 ||
        target.right > innerWidth ||
        target.bottom > innerHeight
      )
        return;
      const reduced = window.matchMedia?.(
        "(prefers-reduced-motion: reduce)",
      ).matches;
      const from =
        !reduced &&
        innerWidth > 640 &&
        entry.current?.getAttribute("aria-expanded") === "false"
          ? origin?.getBoundingClientRect()
          : undefined;
      active.current = true;
      setVisual({ title, target, from: from?.width ? from : undefined });
    } catch {
      /* Keep the caller's static feedback. */
    }
  };
  return (
    <Context.Provider value={{ entry, notify, open }}>
      {children}
      {visual &&
        createPortal(
          <div aria-hidden="true" className="activity-feedback-decoration">
            <span
              className="activity-feedback-pulse"
              style={{
                left: visual.target.left,
                top: visual.target.top,
                width: visual.target.width,
                height: visual.target.height,
              }}
            />
            {visual.from && (
              <span
                className="activity-feedback-flight"
                style={
                  {
                    left: visual.from.left,
                    top: visual.from.top,
                    "--feedback-x": `${visual.target.left - visual.from.left}px`,
                    "--feedback-y": `${visual.target.top - visual.from.top}px`,
                  } as React.CSSProperties
                }
              >
                正在提交 · {visual.title}
              </span>
            )}
          </div>,
          document.body,
        )}
    </Context.Provider>
  );
}

export function SubmissionStatus({
  label,
  busy = false,
  showActivity = false,
  announce = true,
}: {
  label: string;
  busy?: boolean;
  showActivity?: boolean;
  announce?: boolean;
}) {
  const feedback = useActivityFeedback();
  if (!label) return null;
  return (
    <div
      className="submission-status"
      role={announce ? "status" : "note"}
      aria-busy={busy}
    >
      <span>{label}</span>
      {showActivity && (
        <button type="button" onClick={feedback.open}>
          查看活动
        </button>
      )}
    </div>
  );
}
