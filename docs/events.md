# Thread event contract (schema 1)

Thread owns the durable capture boundary. Editors, launchers, agents, and future tools translate their lifecycle into this interface; storage and retrieval do not depend on a vendor SDK.

```sh
thread --vault /path/to/vault event < event.json
```

```json
{
  "schema": 1,
  "id": "editor-session-42-stop-1",
  "source": "my-editor",
  "type": "session.stopped",
  "session_id": "session-42",
  "occurred_at": "2026-09-09T18:00:00Z",
  "project_id": "atlas",
  "machine": "laptop",
  "text": "Paused while investigating cache invalidation. Reproduce with the saved fixture.",
  "references": ["file:///projects/atlas/fixtures/cache.json"],
  "data": {"reason": "user_exit"}
}
```

Required fields: schema `1`, nonblank `id` (at most 512 bytes), nonblank `source` (at most 200 bytes), supported `type`, and RFC3339 `occurred_at`. Session, prompt, and tool events require `session_id`. Optional fields are `project_id`, `repo`, `machine`, `text`, `references`, and an object `data`. `project_id` uses Thread's stable ID syntax. Unknown top-level fields, invalid UTF-8, multiple JSON values, unsupported types, and inputs over 4 MiB are rejected. Put producer-specific evidence in `data`.

Supported types:

```text
session.started     session.stopped    session.idle       session.resumed
prompt.started      prompt.completed  tool.before        tool.after
artifact.created    artifact.modified decision.observed  action.observed
context.changed     project.changed   checkpoint.requested
```

Identity is the pair `(source, id)`. Producers must reuse it for delivery retries and choose a new ID for a new occurrence. Thread compares normalized JSON payloads: object key order and whitespace are irrelevant; changed fields under an existing identity are a conflict. Numeric evidence retains JSON number precision. Successful capture prints the Markdown path after publication and flush. Delivery retries are safe after an uncertain acknowledgment. There is no network service or implicit producer installation.

Events are saved under `Thread/.events/` as YAML properties plus readable, indented original JSON. Original text, timestamps, references, and data survive capture; JSON formatting is normalized. CLI updates reject events and checkpoints. Capture does not read reference files, follow URLs, inspect repositories, invoke AI, or require the project to exist. A supplied project ID links to its eventual project note. Without a project ID, the event remains accessible by `show` or its file but is not included in a project brief. The receiving machine's domain is not assigned to the producer's event.

Occurrence time orders session observations, including delayed delivery. Equal timestamps use stable note IDs as a deterministic tie-breaker, not causal proof. Sessions are grouped by source, machine, and session ID. An observation is not a live lease; missing stop events remain unknown. Corrections use a new event ID and reference the original. This version has no automatic semantic supersession or cross-machine conflict reconciliation.

`checkpoint.requested` captures a request only. Run `thread checkpoint --project ID` separately to materialize a compact summary. This separation keeps capture independent of consolidation failures. Checkpoints retain source links and omitted counts and do not delete underlying evidence. No background worker, retention policy, or vendor-neutral interpretation daemon is installed.

## Existing adapters

Codex CLI 0.153.4 supports native hooks. `thread hook --provider codex --event EVENT` accepts SessionStart, SessionEnd, Stop, Interrupt, UserPromptSubmit, PreToolUse, PostToolUse, PreCompact, and PostCompact. Payloads become immutable events; turn/tool identities distinguish otherwise identical turns, while exact payload retries deduplicate. Prompts and tool evidence are saved during work so an absent stop does not erase earlier observations. SessionStart returns a bounded orientation when its project can be resolved; other hooks produce no stdout. Non-Git directories still capture unassociated events. Transcript paths are retained without reading them. This is durable capture, not complete reconstruction of an interrupted session.

Configure command hooks in `~/.codex/hooks.json` using an absolute tested Thread binary path and review them through Codex `/hooks`. Use synchronous capture, 3-second timeouts for Interrupt/SessionEnd, and 10 seconds for other events. Existing `notify` integrations need not change. New or changed hooks must be trusted by the user before Codex executes them. See [Codex hooks](https://learn.chatgpt.com/docs/hooks).

The Claude hook adapter translates SessionStart, Stop, and SessionEnd to started, idle, and stopped observations, plus UserPromptSubmit, PreToolUse, and PostToolUse to prompt/tool events. It retains its existing local project discovery behavior. Explicit `event_id` and `occurred_at` are accepted; otherwise exact normalized hook payloads deduplicate and receipt time is labeled. Identical occurrences cannot be distinguished without producer delivery IDs. Transcript paths are references only. Assistant messages remain observations, not next-action commitments. Existing legacy session notes are retained.

`thread codex` records normalized start and stop events around the child CLI. An abrupt process or machine termination can leave only the start event. It does not claim visibility into IDE or desktop sessions or individual tools. Neither adapter installs global configuration automatically.

## Interpretation and retrieval

Capture works without a model. Optional `organize --id ID` interprets one capture or an event's original `text` using the existing bounded model runner. Evidence must quote that text exactly; every generated item is a source-linked suggestion. Completed or abandoned outcomes are claims for review, not automatic status changes.

`context --project ID` derives a brief directly from Markdown, without the repository or a hidden index. `--limit` bounds each section; source links and `show --id ID` expose deeper context. Prompt/tool events are counted as omitted and opaque data is excluded. Checkpoints save this reduced view with provenance. Storage currently retains all accepted events, so producers should emit useful boundaries rather than whole transcripts.
