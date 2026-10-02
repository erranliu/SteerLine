# Conversation events v2

Status: the persisted read model now has verified native turn/phase/final/replay support for Windows local Codex and explicitly bound local Orca Codex sessions. Other targets retain their reported limits; see the [capability report](validation/pr-04b-conversation-capabilities-2026-09-24.json) and [acceptance report](validation/pr-04b-conversation-events-2026-09-24.md).

## Model

`ConversationEvent` is the backend-neutral record for the 4C console. It carries project, workflow, execution, attempt, backend, protocol, agent, target, external reference, source segment and source cursor identity. Session, turn, item and source-event IDs each carry `native`, `derived` or `unknown` identity quality. Source byte ranges, sequence/revision, event time when present, receive time and evidence accompany the normalized type.

Each source carries a required `source_role`: `canonical` for verified conversation history and `fallback` for raw or terminal observations. Validation rejects missing or unsupported roles. The event version is 2. Readers reject version 1 events; there is no conversion path for earlier development data.

Events represent `session`, `turn.started`, message delta/snapshot/completed, tool activity, input wait, turn completed/failed/cancelled, observation, gap, structured execution result and unknown source types. `message.completed` closes one message. It does not close its turn. A turn terminal does not identify a final answer. Phase comes from an explicit source field; an absent or unverified phase is `unknown`.

The message item key hashes project, execution, attempt, source segment, session, turn and item identity. A missing item ID stays missing. A turn with an explicit source boundary and no native turn ID uses that boundary's persisted source event ID and the `codex-jsonl-turn-boundary-v1` derivation rule. Output without a reliable turn boundary remains in its source segment.

`FinalAvailability` distinguishes `available`, `final_pending`, `final_absent` and `final_unknown`. Current local and terminal fallback sources do not promote a process result or the last visible text to a final answer. A validated `final_answer` phase may mark content available. Unknown or incomplete source evidence stays unknown; a collected gap is explicit.

## Source mapping

`codex-app-server` is the exact conversation source. The collector uses the public `thread/read` metadata check and `thread/turns/list` with `itemsView=full` and a one-turn page limit. Session, turn and item IDs and `agentMessage.phase` are native fields. It stores full snapshots as immutable revisions and derives replay identities from the native item plus its snapshot. A terminal turn status is accepted only with a non-null native `completedAt`: standalone history reads can synthesize `interrupted` for a still-running turn whose completion was not persisted.

Current local `worker-flow-v1` workers bind a native thread from `thread/start`, a distinct turn from `turn/start`, the fixed worktree and the app-server process generation. Each turn journals WebSocket notifications in `appserver.jsonl`; the existing public history collector supplies complete message bodies and phases. A local Orca Codex launch uses the internal `codex-session-launch` helper in Orca's terminal environment. It creates a session with public app-server calls, persists a bootstrap user item, records the exact session and inherited `CODEX_HOME`, and runs `codex resume` for that ID. The parent binds this record to the verified Orca runtime/terminal/incarnation. The helper does not generate assistant messages or model phases. This route requires an available configured native Codex command (or `node.exe` with the Codex entry script), and a verified local Orca execution host. It neither adopts a supervised Dispatch nor reads a private database.

The service collects bound histories independently of viewers. The source implementation discovers and reads native history, checks the returned session and workspace, and returns mapped writes with a candidate cursor. The service rotates bounded reads across active and historical sources, then publishes that cursor only after the conversation writes commit. It re-reads the last turn anchor to capture updates. A verified native session record is marked canonical; its `source_changed` event names the raw projections it supersedes, and incremental consumers replace those source projections. `ReadConversation` defaults to the canonical source for that attempt, while an explicit `Source` filter retains raw observations and their raw references. A source-read error adds a gap without advancing its cursor.

Native history polling prioritizes the active source and reads at most one ended source per pass. A completed turn without a verified final keeps an anchored revisit cursor, including when newer turns arrive. An unchanged native turn neither creates another event nor advances its source sequence. The implementation returns capability reports tied to the backend, protocol, target, execution, attempt, source identity and observed Codex user agent; host-backed reports include the host's reported version. Host waits remain unverified until a real wait object is observed.

Each public turn cursor is limited to 4 KiB and the persisted history cursor and source binding to 64 KiB. The history client verifies the server-reported Codex home against the binding. If an Orca worktree has since been removed, a separate local directory can host the read-only app-server process; the returned session still has to name the original bound worktree.

The previous local `codex exec --json` source is no longer used by `worker-flow-v1`. Its raw reader remains for other existing adapters. The app-server notification journal is bound to the Command ID, native turn ID and process generation; startup reads it to recover tool requests and matching reply results. Codex `item.updated` message text is stored as a revisioned snapshot; `item.completed` stores its complete item text when present. The mapper retains unknown source events verbatim. Source fields not observed in a real fixture remain unverified in the capability report.

Orca terminal v2 output is read on a collector goroutine bound to the fixed runtime, terminal and incarnation. Terminal bytes are saved as `observation` bodies with their exact output cursor and source. The collector does not parse terminal text into messages or infer turns from words or screen output. A verified Orca reply receipt may create `turn.started` with a derived ID tied to its reply operation. A terminal wait is an `input.wait` event with `wait_origin=host`; it does not create or resolve a SteerLine Decision. Result JSON is `execution.result` with an `execution_summary` body when present, never a conversational final.

## Persistence and reads

Conversation events, immutable body revisions, current message bodies, raw event bytes and source cursors live in the current `steerline.db` SQLite authority. `CommitConversation` writes normalized events, raw bytes, body revisions, dedupe identity and the source checkpoint in one transaction. A source event replay is idempotent. An out-of-order or conflicting revision retains the last known body and adds a diagnostic. Local raw JSONL replay is batched and uses the source file identity to open a new segment after replacement.

The current SQLite schema is 7. Older databases are inspected read-only and rejected; no conversation-table or index upgrade runs on them. Initialize a new schema 7 database for current operation.

The current local checkpoint records an anchored byte position. Startup verifies this format before dispatch, then resumes after the committed byte offset and restores the turn mapper from persisted events. Mapper state is reconstructed from committed session/turn facts for each batch, so a failed commit cannot consume candidate state. It checks file identity, short byte anchors and same-size modification time. An unsupported older checkpoint blocks startup with its original state intact. A changed source opens a new segment with a visible gap; conflicting bytes under one event identity do not advance the cursor. Large raw records beyond 8 MiB produce an explicit gap. Worker output uses bounded backpressure so a full observation queue does not silently drop conversation events.

`ConversationQueryService` exposes `ReadConversation`, `ReadMessageBody` and `ReadRaw`. It returns backend-neutral records and references; callers never supply a file path. Conversation cursors bind project and every query filter. Body reads bind project, execution, attempt, item key and immutable revision.

- Conversation pages default to 200 events, cap at 1,000 and keep encoded event payloads within 1 MiB.
- Message body pages default to 256 KiB and cap at 256 KiB. Page offsets and returned data preserve UTF-8 rune boundaries; every page names the same body revision and SHA-256.
- Raw pages default to and cap at 1 MiB. They return bytes and a content digest under the PR 1 bounded-read contract.
- The event page contains body/raw references for large content. Reading a page does not read arbitrary paths or write runtime/business state.

The existing `activity.jsonl` remains the clipped display derivative described by [Activity contract](activity.md). Conversation history and full result summaries have separate bodies and revisions; deleting or rebuilding an activity view does not change them.

## Console presentation

`steerline console` groups records by project, execution, attempt, source segment, session and turn IDs. It treats `commentary`, tool activity and `unknown` phase as process; only an explicit `final_answer` phase is shown as a final reply. A completed message does not close or collapse its turn. An explicit completed, failed or cancelled turn is handled once by the UI fold reducer; the final reply, terminal reason, gaps, host waits and diagnostics stay visible outside the process fold.

Fold choices and reading anchors live in a versioned UI-owned file under `<STEERLINE_HOME>/ui/views/`. The file contains identities and view choices only. Two active console windows acquire separate view profiles; a later launch reuses an available profile. Corrupt preferences reset the view and leave the business database untouched. Message body pages are read from the exact execution, attempt, item key and body revision; the renderer escapes terminal control characters before displaying text.

When verified turn/phase capability is unavailable, the console identifies the source and uses the `执行过程 / 执行摘要` fallback labels. It does not infer a final reply from the last visible text or infer turn boundaries from quiet time. A turn end does not change waiting-decision, waiting-PR-merge or task completion state.

## Verified scope and remaining gates

E-A9 is accepted for the measured Windows local Codex 0.156.1 and local Orca 1.4.209 Codex targets. Both real two-turn histories passed the same mapper, body-hash and persisted-cursor replay checks; Orca history also contains real tool events. The internal Orca launcher was exercised with two real turns and retained Orca's own Codex environment. The raw `exec --json` and terminal streams retain their original limitations; the verified public history source supplies the missing fields. Remote Orca, other agents and actual host wait/failure/cancellation source samples remain untested. Other fault-injection gaps are listed in the acceptance report.
