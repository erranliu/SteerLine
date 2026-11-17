import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";

export const defaultDirection = "update-stale-docs";

type Direction = { id: string; name: string; prompt: string; revision: string };
type Catalogue = { directions: Direction[]; diagnostics?: string[] };
type DirectionChange =
  | { action: "create"; name: string; prompt: string }
  | {
      action: "update";
      id: string;
      revision: string;
      name: string;
      prompt: string;
    }
  | { action: "delete"; id: string; revision: string };

export function validDirectionPrompt(prompt: string) {
  return (
    !!prompt.trim() &&
    !prompt.includes("\0") &&
    new TextEncoder().encode(prompt).length <= 32 * 1024
  );
}

export function useDirectionCatalogue(enabled: boolean) {
  const { api } = useWorkspace();
  return useQuery({
    queryKey: [api.home, "refactoring-directions"],
    queryFn: ({ signal }) => api.getRefactoringDirections<Catalogue>(signal),
    enabled,
  });
}

export function useDirectionChange() {
  const { api } = useWorkspace();
  const cache = useQueryClient();
  const key = [api.home, "refactoring-directions"];
  return useMutation({
    mutationFn: (change: DirectionChange) =>
      api.changeRefactoringDirection<Direction>(change),
    onSuccess: (direction, change) => {
      cache.setQueryData<Catalogue>(key, (old) => {
        if (!old) return old;
        const id = change.action === "delete" ? change.id : direction.id;
        const directions = old.directions.filter((row) => row.id !== id);
        if (change.action !== "delete") directions.push(direction);
        return { ...old, directions };
      });
    },
    onSettled: () => {
      void cache.invalidateQueries({ queryKey: key });
    },
  });
}

export function DirectionCatalogueStatus({
  catalogue,
}: {
  catalogue: ReturnType<typeof useDirectionCatalogue>;
}) {
  return (
    <>
      {catalogue.isPending && <p role="status">正在读取重构方向</p>}
      {catalogue.isError && <p role="alert">重构方向查询失败</p>}
      {catalogue.data?.diagnostics?.map((message) => (
        <p className="notice amber" role="status" key={message}>
          {message}
        </p>
      ))}
    </>
  );
}

export function ManageDirections({ close }: { close: () => void }) {
  const panel = useRef<HTMLDialogElement>(null);
  const catalogue = useDirectionCatalogue(true);
  const change = useDirectionChange();
  const [selected, setSelected] = useState<Direction>();
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [saved, setSaved] = useState("");
  useEffect(() => {
    if (!panel.current) return;
    if (typeof panel.current.showModal === "function")
      panel.current.showModal();
    else panel.current.setAttribute("open", "");
  }, []);

  function edit(direction?: Direction) {
    setSelected(direction);
    setName(direction?.name ?? "");
    setPrompt(direction?.prompt ?? "");
    setDeleting(false);
    setSaved("");
    change.reset();
  }

  return (
    <dialog
      ref={panel}
      className="refactoring-sheet"
      aria-labelledby="manage-directions-title"
      onCancel={(event) => {
        event.preventDefault();
        if (!change.isPending) close();
      }}
    >
      <section className="refactoring-panel">
        <header className="refactoring-heading">
          <h2 id="manage-directions-title">管理重构方向</h2>
          <Button
            type="button"
            variant="ghost"
            disabled={change.isPending}
            onClick={close}
          >
            关闭
          </Button>
        </header>
        <p className="muted">此安装中的所有项目共享这些方向。</p>
        <DirectionCatalogueStatus catalogue={catalogue} />
        {catalogue.data && (
          <ul className="refactoring-direction-list" aria-label="已保存方向">
            {catalogue.data.directions.map((row) => (
              <li key={row.id}>
                <span>{row.name}</span>
                {row.id === defaultDirection ? (
                  <small className="muted">内置</small>
                ) : (
                  <Button
                    type="button"
                    variant="ghost"
                    disabled={change.isPending}
                    onClick={() => edit(row)}
                    aria-label={`编辑 ${row.name}`}
                  >
                    编辑
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
        <form
          aria-label={selected ? "编辑方向" : "新增方向"}
          onSubmit={(event) => {
            event.preventDefault();
            if (
              change.isPending ||
              deleting ||
              !name.trim() ||
              !validDirectionPrompt(prompt)
            )
              return;
            change.mutate(
              selected
                ? {
                    action: "update",
                    id: selected.id,
                    revision: selected.revision,
                    name,
                    prompt,
                  }
                : { action: "create", name, prompt },
              {
                onSuccess: (row) => {
                  edit(row);
                  setSaved("已保存方向");
                },
              },
            );
          }}
        >
          <fieldset disabled={change.isPending}>
            <legend>{selected ? "编辑方向" : "新增方向"}</legend>
            <label>
              方向名称
              <input
                required
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label>
              方向正文
              <textarea
                required
                rows={7}
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
              />
            </label>
            {prompt && !validDirectionPrompt(prompt) && (
              <p role="status">方向正文必填，最多 32 KiB。</p>
            )}
            {deleting && selected ? (
              <div>
                <p>删除「{selected.name}」？已提交的任务仍使用原正文。</p>
                <Button
                  type="button"
                  onClick={() =>
                    change.mutate(
                      {
                        action: "delete",
                        id: selected.id,
                        revision: selected.revision,
                      },
                      {
                        onSuccess: () => {
                          edit();
                          setSaved("已删除方向");
                        },
                      },
                    )
                  }
                >
                  确认删除
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setDeleting(false)}
                >
                  取消
                </Button>
              </div>
            ) : (
              <div className="refactoring-direction-actions">
                <Button
                  type="submit"
                  disabled={!name.trim() || !validDirectionPrompt(prompt)}
                >
                  保存方向
                </Button>
                {selected && (
                  <>
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => edit()}
                    >
                      新增方向
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => setDeleting(true)}
                    >
                      删除方向
                    </Button>
                  </>
                )}
              </div>
            )}
            {change.error && <p role="alert">{change.error.message}</p>}
            {saved && !change.error && <p role="status">{saved}</p>}
          </fieldset>
        </form>
      </section>
    </dialog>
  );
}
