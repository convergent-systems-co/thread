# Thread

Thread is a Go command line and companion skill for capturing and resuming work across projects. Its backend is an Obsidian vault: Markdown, YAML properties, tags, and wiki links. There is no authoritative hidden database.

Thread stores operational memory for the human across tools and models. It provides deterministic event capture, compact offline orientation, source-linked checkpoints, attention cues, editable work states, relationships, fresh Git inspection, read-only develop/Praxis state imports, optional headless Claude interpretation, Claude lifecycle hooks, a Codex CLI wrapper, and verified local working-file snapshots. Manifold focus control, Copilot adapters, live session leases, automatic import polling, and cross-machine conflict reconciliation remain planned.

## Start

```sh
go build -o bin/thread ./cmd/thread
bin/thread --vault '/absolute/path/to/vault' init
bin/thread --vault '/absolute/path/to/vault' project --id atlas --repo /path/to/atlas --domain home 'Atlas'
```

Set `THREAD_VAULT`, or put `{"vault":"/absolute/path/to/vault"}` in `~/.config/thread/config.json`. An explicit `--vault` overrides both. The vault directory must already exist. Thread adds only its own `Thread/` subtree. Open `Thread/Start Here.md` in Obsidian; enable the core Bases plugin if the embedded view does not appear.

```sh
thread capture 'Customer review needs X, Y, and Z'
thread capture --project atlas --kind action --status active --next 'Ask Dana which format is required' 'Export requirements'
thread resume --repo /path/to/atlas
thread event < event.json
thread context --project atlas
thread checkpoint --project atlas
thread status --domain home
thread set --id RECORD_ID --status paused --next 'Run the integration test'
thread link --from RECORD_ID --to OTHER_RECORD_ID
thread dashboard [--domain home|work|unknown]
thread hook --provider claude --event session-start < hook-payload.json
thread extract --repo /path/to/heimdall/projects --source heimdall --domain home
thread skill install --client all
```

Flags precede positional text. `capture --stdin` reads multiline text without placing it in shell arguments. `show --id ID --json` and `resume --json` support agent/tool integrations.

Project domain overrides machine defaults. Odin defaults to home, Hindal to work; other hosts remain unknown. This release stores one registered local repository path per project; other machines need a future location mapping rather than overwriting paths back and forth. Explicit project IDs and Git common-directory matching keep worktrees associated with the same project. No Git remote is contacted.

`thread extract` inventories repositories under a root you explicitly provide. `--source` is the machine name (for example `heimdall`) and `--domain home` marks the extracted projects as personal. It scans only a bounded local directory tree, follows no symlinks, and never clones, moves, edits, or deletes repositories. Existing projects are deduplicated by Git common directory. Thread cannot inspect an unreachable machine by name alone.

## Operational memory

Capture, interpretation, consolidation, retrieval, and attention management are separate operations. `thread event` captures one provider-neutral event without AI, repository inspection, or project registration. Producers supply stable delivery IDs so retries are idempotent; conflicting payloads retain the original and return an error. See the [event contract](docs/events.md) for the JSON interface and adapter behavior.

`thread context --project atlas [--limit 5] [--json]` returns a bounded brief: return cues, references, suggestions, observations, last observed sessions, and reasons something needs attention or can be set down. It works when the repository or original machine is unavailable. Each section contains at most the requested number of cues (1–20), with omitted counts and source links for deeper retrieval through `show` or Obsidian. Prompt/tool details and opaque provider data stay out of the brief. `resume` retains its existing repository inspection behavior.

`thread checkpoint --project atlas` consolidates the brief into an immutable Markdown note. Identical views deduplicate; changed views create a new checkpoint. Checkpoints are historical, bounded summaries with links to evidence. Original notes remain available; there is no automatic deletion or retention worker.

Attention cues use recorded evidence: a paused item with a return cue can be set down; blockers and missing return cues need attention. Saved suggestions remain unaccepted. An idle/stopped session establishes only a captured lifecycle boundary. A start without a later boundary means current activity is unknown. These cues neither schedule reminders nor establish completion or code recovery.

Additional capture kinds are `assumption`, `question`, `constraint`, `artifact`, `dependency`, `risk`, `direction`, and `outcome`. Completed or abandoned work can be explicitly recorded with `done` or `archived`, preserving the explanation in the body. Optional `organize` proposes these kinds with exact supporting evidence; every proposal remains `suggested`. It accepts captures or a normalized event's `text`; opaque event data and referenced files are not sent to the model.

## Existing execution systems

```sh
thread import --provider develop --file /path/to/run/state.json --project atlas
thread import --provider praxis --file /path/to/praxis/state.json --project atlas
thread dashboard
```

Imports never advance, resume, or modify the source run. Changed source bytes produce a separate immutable note; repeated identical imports are idempotent. The latest observation per source path is used in metrics, avoiding double-counting from repeated polling. Separate runs still count separately, including any task attempts they repeat. Source paths, hashes, observation timestamps, and available source timestamps are retained. A snapshot can be stale immediately after import; it is not a live monitor.

The develop adapter uses runtime cursors for bundle state and records task completion, handoffs, repair cycles when present, concerns, and handoff locations. Praxis uses its cursor statuses and records successful nodes. These are workflow measurements, not human productivity scores. Unreported tokens, duration, costs, and effort are unknown.

## Optional headless organization

```sh
thread organize --id CAPTURE_ID
```

This sends only the selected capture to the installed, authenticated Claude CLI. It runs noninteractively with no tools/MCP, in an empty temporary directory, using `--safe-mode` to skip personal/repository customizations while retaining normal login authentication. Managed policy remains in effect. It does not bypass permissions or open a new UI window. A 90-second timeout and bounded output constrain the invocation. Authentication and billing remain with Claude. The smoke test caught that `--bare` does not retain the same authentication behavior, so it is deliberately not used.

The original capture is never modified. Results must include exact supporting excerpts from the capture and are saved as linked **suggestions**, never automatically promoted to commitments. Identical proposals deduplicate; different rerun proposals can coexist. The command reports errors, retaining the original on provider failure. Classification is explicitly invoked in this release; there is no always-on worker. The provider boundary is covered by fixtures and passed a live synthetic-capture smoke test with Claude Code 2.1.263 on Odin; availability on other machines depends on their CLI/login.

Verified against the [Claude CLI reference](https://code.claude.com/docs/en/cli-reference). Additional providers can implement the same bounded JSON-returning interface.

## Recovery

```sh
thread snapshot --repo /path/to/atlas --project atlas
thread verify --file /absolute/path/to/recovery.zip
```

Snapshots write a new private ZIP under `~/.local/state/thread/recovery/`, outside the vault. They include existing tracked and nonignored untracked regular files, with modes and SHA-256 checksums. Deleted tracked files are absent from the complete working-file inventory. Each archive is read back and verified before it is published, and a linked memory records its location and scope. Snapshot creation never commits, stashes, or writes to the repository.

The archive **excludes Git history and index state, ignored files, unsaved editor buffers, symlinks, and submodule contents**. It is a local recovery copy, not disaster recovery or a complete repository backup. Files can contain sensitive data, including tracked secrets; archives stay local and are not uploaded by Thread. Files/source state are rechecked during capture, but there is no filesystem-wide point-in-time snapshot; pause concurrent writers for a coherent capture. The archive size limit is 128 MiB of regular-file content. To recover, verify the archive and extract into a **new directory**, inspecting its manifest before using its files. Never extract over a live checkout by default.

## Storage and Obsidian

```text
Thread/
  Start Here.md       Entry point with editable Bases views
  Thread.base         Views of current items and projects
  Overview.md         Generated overview; refresh with thread dashboard
  Guide.md            In-vault usage notes
  Projects/           Stable project IDs
  .items/             Thread-managed captures, actions, decisions, memory, habits, recovery notes
  .runs/              Thread-managed immutable execution observations
  .events/            Immutable provider-neutral lifecycle and evidence events
  .history/           Prior versions of CLI-edited notes
```

Edit item `status`, `next`, `tags`, `related`, and other properties directly in Obsidian. The CLI retains unknown properties and the Markdown body on update; formatting of frontmatter may change. Keep IDs and required schema fields intact. Project and related wiki links feed backlinks and the graph. Inferred relationships should be proposed with supporting evidence; `link` records an explicit relationship, whose meaning can be described in the body.

Creation flushes a temporary file before exclusive publication. CLI updates take a per-note local lock, preserve original bytes in `.history`, and detect external changes before replacement. A lock file left by a crash is surfaced for inspection, never silently broken. These controls do not implement distributed locking across iCloud devices. Avoid simultaneous edits of the same note; duplicate IDs and malformed Markdown raise errors rather than silently hiding data. Symlinks in managed storage are rejected. Keep independent vault backups and let iCloud finish syncing before switching devices.

On macOS, an iCloud/FileProvider vault may retain filenames and sizes while evicting file content. Thread detects these dataless records before reading them. `resume` skips them, reports that its result may be incomplete, and includes a `vault_warning` in JSON output; commands that require a complete vault fail clearly. All vault scans have a five-second read deadline. Lifecycle hooks have a separate three-second deadline and fail open with a `Thread hook capture skipped` message so a sync stall cannot hold an AI client session open.

`Overview.md` is generated; it is replaced only if its ownership marker is present. `thread dashboard` refreshes that Markdown file and prints its path; it does not open Obsidian. Add `--domain home`, `--domain work`, or `--domain unknown` to generate a filtered view. Other user-owned notes are not rendered over. The selected vault's mobile location and actual iCloud propagation must be checked on the user's devices; local writes do not prove phone sync.

## Companion skill

`skills/thread/SKILL.md` guides capture, orientation, stopping-point memory, and recovery through the CLI. The release binary embeds a compact copy and can install it with `thread skill install --client codex|claude|all`. It refuses to replace changed files unless `--force` is supplied. Copilot is not included in `all`: its skill discovery path and repository/user scope need a separate explicit adapter. The Claude lifecycle adapter is opt-in; no global hook configuration is installed automatically.

## Install released binaries

```sh
brew tap convergent-systems-co/tap
brew install convergent-systems-co/tap/thread
```

Windows x64 can install the portable release through Winget after its manifest is accepted:

```powershell
winget install --id ConvergentSystemsCo.Thread
```

The current manifest is kept under `packaging/winget/`. It is not claimed to be in the public Winget source until Microsoft accepts the submission.

## Development

```sh
go test -race ./...
go vet ./...
```

Tagged releases notify `convergent-systems-co/homebrew-tap` after GoReleaser publishes every artifact. Configure the Thread repository Actions secret `HOMEBREW_TAP_TOKEN` with a fine-grained GitHub token that has Contents read/write access to the tap repository; the release workflow fails visibly if this credential is missing instead of silently leaving Homebrew stale.

Tests cover concurrent capture, duplicate creation, user-content preservation, note history, malformed/conflicting records, path escapes, idempotent imports, metrics aggregation, source status names, unsupported AI claims, provider failure, and recovery of uncommitted/untracked bytes without changing Git state.
