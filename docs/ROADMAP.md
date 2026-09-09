# Workstation integration plan

The first release establishes the durable backend and CLI. Track these as proposed work, not already delivered functionality.

1. Lifecycle adapters for the specific installed Codex/Claude/Copilot surfaces. Capture incrementally, show orientation at startup, report capture coverage, survive abrupt termination, and suppress internal classifier recursion. Test each provider separately.
2. Read-only observers for develop/Praxis run updates. Preserve source cursors and authority. Map task/work identities across repeated runs; support source event IDs rather than counting repeated attempts as distinct completed work.
3. Live session registrations scoped to machine, repository, worktree, process incarnation, provider session ID, and optional Manifold deployment ID. Expiring presence distinguishes idle/missing sessions from known-live ones. Same-project notices differ from same-working-directory warnings.
4. Manifold local control bridge: resolve a Thread work item, focus an associated live pane before opening a new one, show project/branch/objective in pane chrome, and restore broadcasting inactive. Do not feed prompts to an arbitrary active pane. Preserve Thread operation without Manifold.
5. Headless organization queue with retry/backoff, source hashing, provider adapters, classification coverage, bounded batches, and per-domain handling. Learn explicit preferences; do not silently promote inferred habits or new obligations.
6. Recovery expansion: full Git history/index, retention, external backups, restore drills, and filesystem-consistent snapshots. Make excluded or unprotected work visible per worktree.
7. Cross-machine location mapping, concurrent edit reconciliation, mobile capture, and live Obsidian dashboard updates. Validate the actual iCloud/mobile path before claiming phone access.
8. Relationship proposals backed by cited notes and source evidence; measures of time-to-resume, rework, repeated blockers, and completions with coverage/definitions. Keep inferred human effort separate from agent activity.

Acceptance scenario: capture a request during a call; do related work; close the assistant; reopen the repository on the correct machine; see the verified stopping point; focus the correct Manifold pane or create it once; recover recorded working files into a clean directory if necessary.
