import type { Envelope, Receipt, StoredRequest } from "./types";

let database: Promise<IDBDatabase> | undefined;
export function openJournal() {
  return (database ??= new Promise<IDBDatabase>((resolve, reject) => {
    const req = indexedDB.open("steerline-web", 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore("requests", {
        keyPath: "envelope.request_id",
      });
      req.result.createObjectStore("drafts");
    };
    req.onsuccess = () => {
      req.result.onversionchange = () => {
        req.result.close();
        database = undefined;
      };
      resolve(req.result);
    };
    req.onerror = () => {
      database = undefined;
      reject(req.error);
    };
    req.onblocked = () => {
      database = undefined;
      reject(new Error("本机存储被其他标签页阻塞，请关闭旧页面后重试"));
    };
  }));
}
export function canonical(value: unknown): string {
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (value && typeof value === "object")
    return (
      "{" +
      Object.entries(value)
        .filter(([, v]) => v !== undefined)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([k, v]) => JSON.stringify(k) + ":" + canonical(v))
        .join(",") +
      "}"
    );
  return JSON.stringify(value);
}
async function transaction<T>(
  store: string,
  mode: IDBTransactionMode,
  work: (object: IDBObjectStore, done: (value: T) => void) => void,
): Promise<T> {
  const db = await openJournal();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, mode);
    let result: T;
    tx.oncomplete = () => resolve(result);
    tx.onerror = () => reject(tx.error || new Error("本机存储失败"));
    tx.onabort = () => reject(tx.error || new Error("本机存储事务取消"));
    try {
      work(tx.objectStore(store), (value) => {
        result = value;
      });
    } catch (error) {
      tx.abort();
      reject(error);
    }
  });
}
export async function saveRequest(envelope: Envelope) {
  return transaction<StoredRequest>("requests", "readwrite", (store, done) => {
    const req = store.get(envelope.request_id);
    req.onsuccess = () => {
      const old = req.result as StoredRequest | undefined;
      if (old) {
        if (canonical(old.envelope) !== canonical(envelope)) {
          store.transaction.abort();
          return;
        }
        done(old);
      } else {
        const row = {
          envelope: structuredClone(envelope),
          created_at: new Date().toISOString(),
        };
        store.add(row);
        done(row);
      }
    };
  });
}
export async function requestByID(id: string) {
  return transaction<StoredRequest | undefined>(
    "requests",
    "readonly",
    (store, done) => {
      const req = store.get(id);
      req.onsuccess = () => done(req.result);
    },
  );
}
export async function listRequests() {
  return transaction<StoredRequest[]>("requests", "readonly", (store, done) => {
    const req = store.getAll();
    req.onsuccess = () => done(req.result);
  });
}
export async function recordResult(
  id: string,
  response?: Receipt,
  error?: string,
) {
  return transaction<void>("requests", "readwrite", (store, done) => {
    const req = store.get(id);
    req.onsuccess = () => {
      const old = req.result as StoredRequest | undefined;
      if (!old) {
        store.transaction.abort();
        return;
      }
      const terminal =
        old.response?.status === "applied" ||
        old.response?.status === "rejected";
      if (!terminal)
        store.put({ ...old, response: response ?? old.response, error });
      done();
    };
  });
}
export async function readDraft(key: string) {
  return transaction<string>("drafts", "readonly", (store, done) => {
    const req = store.get(key);
    req.onsuccess = () => done(req.result ?? "");
  });
}
export async function writeDraft(key: string, value: string) {
  return transaction<void>("drafts", "readwrite", (store, done) => {
    store.put(value, key);
    done();
  });
}
