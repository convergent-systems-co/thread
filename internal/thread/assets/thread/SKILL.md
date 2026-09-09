---
name: thread
description: Capture commitments, decisions, discoveries, and working preferences in the configured Obsidian vault; orient or resume a registered project and record where work stopped.
---

# Thread

Use `thread resume --repo /absolute/current/worktree --json` at the start of work. If the repository is unregistered, Claude lifecycle hooks create a paused project shell automatically so the session is not lost; confirm its title, domain, and ownership before activating it. Present the project, domain, worktree/branch, recorded stopping point, one next action, and source freshness. Verify current Git state before asserting completion.

Use `thread capture --project ID --kind action|decision|discovery|habit|memory` for meaningful work memory. Preserve original wording and evidence. AI proposals stay `suggested` until the user adopts them. Use `thread set` for state changes and `thread link` for explicit relationships.

Before leaving meaningful work, record what changed, what was verified, what remains uncertain, and one concrete next action. `thread snapshot --repo PATH --project ID` creates a verified local working-file ZIP, but excludes Git history/index, ignored files, unsaved editor buffers, symlinks, and submodules.

Use `thread import --provider develop|praxis` for read-only execution observations. Use `thread hook --provider claude` only when configured by the user. Do not claim that Thread has focused a Manifold pane or captured an unsupported client lifecycle.
