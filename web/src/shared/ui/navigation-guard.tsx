import { useContext, useEffect } from "react";
import {
  UNSAFE_DataRouterContext,
  useBeforeUnload,
  useBlocker,
} from "react-router-dom";

const message = "当前修改尚未保存，是否放弃修改并离开？";

export function NavigationGuard({ dirty }: { dirty: boolean }) {
  const router = useContext(UNSAFE_DataRouterContext);
  useBeforeUnload((event) => {
    if (dirty) {
      event.preventDefault();
      event.returnValue = "";
    }
  });
  return router ? <RouterGuard dirty={dirty} /> : null;
}

function RouterGuard({ dirty }: { dirty: boolean }) {
  const blocker = useBlocker(dirty);
  useEffect(() => {
    if (blocker.state === "blocked") {
      if (window.confirm(message)) blocker.proceed();
      else blocker.reset();
    }
  }, [blocker]);
  return null;
}
