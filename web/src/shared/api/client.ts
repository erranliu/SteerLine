import {
  canonical,
  saveRequest,
  discardUnsentRequest,
  recordResult,
  requestByID,
  listRequests,
  setJournalHome,
} from "./journal";
import type {
  Envelope,
  Identity,
  Receipt,
  BodyRef,
  FinalReply,
  Conversation,
  BrowserSession,
} from "./types";

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
function safeNumbers(value: unknown): void {
  if (
    typeof value === "number" &&
    Number.isInteger(value) &&
    !Number.isSafeInteger(value)
  )
    throw new Error("接口整数超出浏览器精确范围，不能提交目标版本");
  if (value && typeof value === "object")
    Object.values(value).forEach(safeNumbers);
}
export async function json<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    cache: "no-store",
    headers: { Accept: "application/json", ...init.headers },
  });
  let body;
  try {
    body = await response.json();
  } catch {
    throw new ApiError(
      "invalid_response",
      "服务未返回有效 JSON",
      response.status,
    );
  }
  if (!response.ok)
    throw new ApiError(
      body.code ?? "request_failed",
      body.reason ?? "请求失败",
      response.status,
    );
  safeNumbers(body);
  return body as T;
}
export async function authorize() {
  const fragment = new URLSearchParams(location.hash.slice(1));
  const ticket = fragment.get("ticket");
  if (ticket) {
    history.replaceState(null, "", location.pathname + location.search);
    await json("/auth/session", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ticket }),
    });
  }
  const session = await json<BrowserSession>("/auth/status");
  setJournalHome(session.identity.home_id);
  return session;
}
export function matches(receipt: Receipt, envelope: Envelope) {
  return (
    receipt.request_id === envelope.request_id &&
    receipt.project_id === envelope.project_id &&
    receipt.instance_id === envelope.instance_id &&
    receipt.action === envelope.action &&
    receipt.target_id === envelope.target_id &&
    receipt.expected_version === envelope.expected_version &&
    receipt.original !== undefined &&
    canonical(receipt.original) === canonical(envelope) &&
    [
      "submitted",
      "accepted",
      "applied",
      "rejected",
      "outcome_unknown",
    ].includes(receipt.status)
  );
}
export class Api {
  constructor(
    public identity: Identity,
    public csrf: string,
    public home = "",
  ) {}
  private active = true;
  private lifetime = new AbortController();
  activate() {
    this.active = true;
    if (this.lifetime.signal.aborted) this.lifetime = new AbortController();
  }
  deactivate() {
    this.active = false;
    this.lifetime.abort();
  }
  private check() {
    if (!this.active) throw new DOMException("项目已切换", "AbortError");
  }

  path(route: string) {
    return "/api/v2/application/projects/" + this.identity.project_id + route;
  }
  get<T>(route: string, signal?: AbortSignal) {
    this.check();
    return json<T>(this.path(route), {
      signal: signal
        ? AbortSignal.any([signal, this.lifetime.signal])
        : this.lifetime.signal,
    });
  }
  envelope(
    action: string,
    target_id: string,
    expected_version: number,
    payload: Record<string, unknown>,
    extra: Partial<Envelope> = {},
  ): Envelope {
    return {
      ...extra,
      request_id: crypto.randomUUID().replaceAll("-", ""),
      project_id: this.identity.project_id,
      instance_id: this.identity.instance_id,
      action,
      target_id,
      expected_version,
      payload,
    };
  }
  private async locked<T>(id: string, work: () => Promise<T>) {
    if (!navigator.locks)
      throw new Error(
        "当前浏览器不支持原请求发送锁，请使用当前版本 Edge/Chrome",
      );
    return navigator.locks.request(
      "steerline-request-" + this.home + "/" + id,
      work,
    );
  }
  private validate(envelope: Envelope) {
    this.check();
    if (
      envelope.project_id !== this.identity.project_id ||
      envelope.instance_id !== this.identity.instance_id
    )
      throw new Error("原请求属于其他项目或服务实例，请保留历史并重新核对");
    safeNumbers(envelope);
  }
  private async post(envelope: Envelope) {
    this.validate(envelope);
    const result = await json<Receipt>(this.path("/commands"), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": this.csrf,
      },
      body: JSON.stringify(envelope),
    });
    if (!matches(result, envelope))
      throw new Error("回执与原请求身份或正文不匹配，保留原请求待核实");
    await recordResult(envelope.request_id, result);
    return result;
  }
  async submit(envelope: Envelope) {
    envelope = structuredClone(envelope);
    this.validate(envelope);
    const submit = () =>
      this.locked(envelope.request_id, async () => {
        this.check();
        // Each confirmed additional candidate is an independent operation.
        // All other actions retain their per-target uncertainty guard.
        const pending =
          envelope.action !== "start_candidate" &&
          (await listRequests()).find(
            (row) =>
              row.envelope.request_id !== envelope.request_id &&
              row.envelope.project_id === envelope.project_id &&
              row.envelope.instance_id === envelope.instance_id &&
              row.envelope.action === envelope.action &&
              row.envelope.target_id === envelope.target_id &&
              (!row.response ||
                !["applied", "rejected"].includes(row.response.status)),
          );
        if (pending)
          throw new Error(
            "原目标已有待核实操作，请查询原请求，不能创建替代请求",
          );
        const old = await requestByID(envelope.request_id);
        if (old) return this.reconcileUnlocked(envelope);
        this.check();
        await saveRequest(envelope);
        if (!this.active) {
          await discardUnsentRequest(envelope);
          throw new DOMException("项目已切换", "AbortError");
        }
        try {
          return await this.post(envelope);
        } catch (error) {
          await recordResult(envelope.request_id, undefined, String(error));
          throw error;
        }
      });
    if (envelope.action === "start_candidate") return submit();
    return this.locked(
      "target/" +
        envelope.project_id +
        "/" +
        envelope.instance_id +
        "/" +
        envelope.action +
        "/" +
        envelope.target_id,
      submit,
    );
  }
  async reconcile(envelope: Envelope, signal?: AbortSignal) {
    envelope = structuredClone(envelope);
    this.check();
    if (envelope.project_id !== this.identity.project_id)
      throw new Error("原请求属于其他项目或服务实例");
    safeNumbers(envelope);
    return this.locked(envelope.request_id, () => {
      signal?.throwIfAborted();
      return this.reconcileUnlocked(envelope, signal);
    });
  }
  private async reconcileUnlocked(
    envelope: Envelope,
    signal?: AbortSignal,
  ): Promise<Receipt> {
    const stored = await requestByID(envelope.request_id);
    if (!stored || canonical(stored.envelope) !== canonical(envelope))
      throw new Error("找不到匹配的完整原信封，禁止补交");
    if (
      stored.response?.status === "applied" ||
      stored.response?.status === "rejected"
    )
      return stored.response;
    try {
      const result = await this.get<Receipt>(
        "/commands/" + envelope.request_id,
        signal,
      );
      signal?.throwIfAborted();
      if (!matches(result, envelope)) throw new Error("对账回执与原信封不匹配");
      await recordResult(envelope.request_id, result);
      return result;
    } catch (error) {
      if (
        error instanceof ApiError &&
        error.code === "command_not_found" &&
        envelope.action !== "start_environment_check" &&
        envelope.action !== "stop_environment_check" &&
        !envelope.action.startsWith("maintenance_")
      ) {
        signal?.throwIfAborted();
        if (envelope.instance_id !== this.identity.instance_id)
          throw new Error(
            "原请求属于其他项目或服务实例，保留原信封，不能向新实例补交",
          );
        return this.post(envelope);
      }
      if (signal?.aborted) throw error;
      await recordResult(envelope.request_id, undefined, String(error));
      throw error;
    }
  }
  // Explicit continuation of an already durable single-file intent. Never called
  // by queries, polling, reconnect or request-journal restoration.
  async continueMaintenanceApply(envelope: Envelope) {
    this.validate(envelope);
    if (envelope.action !== "maintenance_apply")
      throw new Error("只能显式继续原文件应用");
    return this.locked(envelope.request_id, async () => {
      const row = await requestByID(envelope.request_id);
      if (!row || canonical(row.envelope) !== canonical(envelope))
        throw new Error("缺少完整原请求");
      const current = await this.reconcileUnlocked(envelope);
      if (current.status !== "outcome_unknown") return current;
      return this.post(envelope);
    });
  }
  async final(
    run: { run_id: string; execution_id: string; attempt_id: string },
    signal?: AbortSignal,
  ): Promise<FinalReply> {
    try {
      const reply = await this.get<FinalReply>(
        "/runs/" + run.run_id + "/final",
        signal,
      );
      if (
        reply.workflow_id !== run.run_id ||
        reply.execution_id !== run.execution_id ||
        reply.attempt_id !== run.attempt_id
      )
        throw new Error("最终回复目标已变化");
      if (!reply.page.has_more) return reply;
    } catch (error) {
      if (!(error instanceof ApiError) || error.code !== "response_too_large")
        throw error;
    }
    const bodies: FinalReply["bodies"] = [];
    const events: Conversation["events"] = [];
    const cursors = new Set<string>();
    let cursor = "";
    do {
      if (cursors.has(cursor)) throw new Error("最终回复分页游标重复");
      cursors.add(cursor);
      const response = await this.get<{ page: Conversation }>(
        "/runs/" +
          run.run_id +
          "/conversation?" +
          new URLSearchParams({
            execution_id: run.execution_id,
            attempt_id: run.attempt_id,
            finals_only: "true",
            limit: "50",
            cursor,
          }),
        signal,
      );
      events.push(...response.page.events);
      for (const event of response.page.events)
        if (event.body) {
          if (
            event.body.execution_id !== run.execution_id ||
            event.body.attempt_id !== run.attempt_id
          )
            throw new Error("最终回复正文属于其他执行或 Attempt");
          bodies.push(await this.body(run.run_id, event.body, signal));
        }
      if (response.page.has_more && !response.page.next_cursor)
        throw new Error("最终回复分页没有继续游标");
      cursor = response.page.has_more ? (response.page.next_cursor ?? "") : "";
    } while (cursor);
    return {
      workflow_id: run.run_id,
      execution_id: run.execution_id,
      attempt_id: run.attempt_id,
      bodies,
      page: { events, has_more: false },
    };
  }
  private async body(runID: string, ref: BodyRef, signal?: AbortSignal) {
    const encoded = Array.from(
      new TextEncoder().encode(JSON.stringify(ref)),
      (byte) => byte.toString(16).padStart(2, "0"),
    ).join("");
    const chunks: Uint8Array[] = [];
    let offset = 0;
    let total = 0;
    do {
      const page = await this.get<{
        item_key: string;
        revision: number;
        sha256: string;
        offset: number;
        next_offset: number;
        total_bytes: number;
        data: string;
        complete: boolean;
      }>(
        "/runs/" +
          runID +
          "/body?" +
          new URLSearchParams({
            ref: encoded,
            offset: String(offset),
            limit: "262144",
          }),
        signal,
      );
      if (
        page.item_key !== ref.item_key ||
        page.revision !== ref.revision ||
        page.sha256 !== ref.sha256 ||
        page.offset !== offset ||
        page.total_bytes !== ref.bytes ||
        page.next_offset < offset ||
        page.next_offset > ref.bytes ||
        (page.next_offset === offset && page.total_bytes !== offset)
      )
        throw new Error("正文身份/分页已变化");
      const bytes = Uint8Array.from(atob(page.data ?? ""), (c) =>
        c.charCodeAt(0),
      );
      if (bytes.length !== page.next_offset - offset)
        throw new Error("正文片段长度不匹配");
      chunks.push(bytes);
      total += bytes.length;
      offset = page.next_offset;
    } while (offset < ref.bytes);
    const joined = new Uint8Array(total);
    let position = 0;
    for (const chunk of chunks) {
      joined.set(chunk, position);
      position += chunk.length;
    }
    const digest = Array.from(
      new Uint8Array(await crypto.subtle.digest("SHA-256", joined)),
      (b) => b.toString(16).padStart(2, "0"),
    ).join("");
    if (digest !== ref.sha256) throw new Error("正文 SHA 校验失败");
    return {
      item_key: ref.item_key,
      revision: ref.revision,
      text: new TextDecoder("utf-8", { fatal: true }).decode(joined),
      complete: ref.completeness === "complete",
      completeness: ref.completeness,
    };
  }
}
