import type { Api } from "../../shared/api/client";

export type ReadingItem = {
  id: string;
  role: string;
  kind: string;
  label: string;
  preview: string;
  bytes: number;
  sha256: string;
  at?: string;
  state?: string;
  run_id?: string;
  attempt_id?: string;
  session_id?: string;
  turn_id?: string;
  group: string;
};
export type ReadingPage = {
  activity_id: string;
  items: ReadingItem[];
  version: string;
  has_more: boolean;
  next_cursor?: string;
  diagnostic: string;
};
export type ReadingBody = {
  id: string;
  sha256: string;
  offset: number;
  next_offset: number;
  bytes: number;
  text: string;
  complete: boolean;
};
export const readingRoot = (id: string) =>
  "/activities/" + encodeURIComponent(id);
export async function readInteractions(
  api: Api,
  id: string,
  cursor: string,
  signal: AbortSignal,
) {
  const query = new URLSearchParams({ limit: "50" });
  if (cursor) query.set("cursor", cursor);
  const page = await api.get<ReadingPage>(
    readingRoot(id) + "/interactions?" + query,
    signal,
  );
  if (
    page.activity_id !== id ||
    !Array.isArray(page.items) ||
    (page.has_more && !page.next_cursor) ||
    !page.version
  )
    throw new Error("交互索引范围或分页无效");
  return page;
}
export async function readBody(
  api: Api,
  id: string,
  item: ReadingItem,
  offset: number,
  signal: AbortSignal,
) {
  const query = new URLSearchParams({
    item: item.id,
    sha256: item.sha256,
    offset: String(offset),
  });
  const page = await api.get<ReadingBody>(
    readingRoot(id) + "/body?" + query,
    signal,
  );
  const bytes = new TextEncoder().encode(page.text).length;
  if (
    page.id !== item.id ||
    page.sha256 !== item.sha256 ||
    page.bytes !== item.bytes ||
    page.offset !== offset ||
    bytes > 256 * 1024 ||
    page.next_offset !== offset + bytes ||
    page.next_offset > item.bytes ||
    page.complete !== (page.next_offset === item.bytes) ||
    (!page.complete && bytes === 0)
  )
    throw new Error("正文分段与原引用不符");
  return page;
}
