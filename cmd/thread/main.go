package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	core "github.com/convergent-systems-co/thread/internal/thread"
)

var version = "dev"

const usage = `Thread — capture and resume work through an Obsidian vault.

Usage: thread [--vault PATH] COMMAND [flags] [text]

  init                         Add Thread's views and guide (preserves existing files)
  project --id ID --repo PATH --domain home|work TITLE
  capture [--project ID] [--kind action|decision|discovery|habit|memory] TEXT
          [--status inbox|suggested|active|paused|blocked|done|archived]
          [--next TEXT] [--tags a,b] [--source manual|assistant] [--stdin]
  set --id ID [--status STATE] [--next TEXT] [--domain home|work]
  link --from ID --to ID        Add an explicit relationship
  resume [--project ID] [--repo PATH] [--json]
  event                        Capture one normalized event JSON object from stdin
  context --project ID [--limit 5] [--json]
                               Read a compact operational brief, even offline
  checkpoint --project ID      Save a source-linked operational brief
  status [--domain home|work]    Portfolio overview in the terminal
  show --id ID [--json]         Read one record
  import --provider develop|praxis --file STATE_JSON --project ID
  dashboard [--domain home|work|unknown]
                               Refresh the generated Obsidian overview
  organize --id ID             Extract suggestions from one capture using headless Claude
  snapshot --repo PATH         Preserve working files in a verified local ZIP
  verify --file ZIP            Verify archived files against recorded checksums
  extract --repo PATH --source MACHINE --domain home|work
                               Register Git repositories under a machine root
  hook --provider claude|codex --event EVENT
                               Consume one native hook JSON payload from stdin
  codex [--] [CODEX_ARGS]
                               Run Codex CLI with Thread lifecycle capture
  skill install --client codex|claude|all [--force]
                               Install the embedded Thread skill for an AI client

Put flags before positional text. THREAD_VAULT supplies the vault by default.
This foundation does not run development tasks or claim to back up your code.
`

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(version)
			return
		}
	}
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(version)
			return
		}
	}
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "thread:", err)
		os.Exit(1)
	}
}
func run(args []string, in io.Reader, out io.Writer) error {
	global := flag.NewFlagSet("thread", flag.ContinueOnError)
	global.SetOutput(out)
	defaultVault := os.Getenv("THREAD_VAULT")
	if defaultVault == "" {
		home, _ := os.UserHomeDir()
		b, e := os.ReadFile(filepath.Join(home, ".config", "thread", "config.json"))
		if e == nil {
			var config struct {
				Vault string `json:"vault"`
			}
			if e = json.Unmarshal(b, &config); e != nil {
				return fmt.Errorf("invalid Thread configuration: %w", e)
			}
			defaultVault = config.Vault
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	vault := global.String("vault", defaultVault, "Obsidian vault")
	if err := global.Parse(args); err != nil {
		return err
	}
	args = global.Args()
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprint(out, usage)
		return nil
	}
	if args[0] == "version" {
		fmt.Fprintln(out, version)
		return nil
	}
	if args[0] == "skill" {
		if len(args) < 2 || args[1] != "install" {
			return errors.New("usage: thread skill install --client codex|claude|all [--force]")
		}
		sk := flag.NewFlagSet("skill install", flag.ContinueOnError)
		sk.SetOutput(out)
		client := sk.String("client", "", "AI client")
		force := sk.Bool("force", false, "replace changed files")
		if err := sk.Parse(args[2:]); err != nil {
			return err
		}
		paths, err := core.InstallSkill(*client, *force)
		for _, p := range paths {
			fmt.Fprintln(out, p)
		}
		return err
	}
	if args[0] == "version" {
		fmt.Fprintln(out, version)
		return nil
	}
	s, err := core.Open(*vault)
	if err != nil {
		return err
	}
	if args[0] == "codex" {
		return s.RunCodex(context.Background(), args[1:], in, out, os.Stderr)
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.SetOutput(out)
	id := f.String("id", "", "record/project ID")
	project := f.String("project", "", "project ID")
	repo := f.String("repo", "", "repository path")
	kind := f.String("kind", "capture", "record kind")
	status := f.String("status", "", "record status")
	next := f.String("next", "", "next action")
	domain := f.String("domain", "", "home, work, unknown")
	source := f.String("source", "manual", "source")
	tags := f.String("tags", "", "comma-separated tags")
	stdin := f.Bool("stdin", false, "read body from stdin")
	asJSON := f.Bool("json", false, "JSON output")
	provider := f.String("provider", "", "import provider")
	eventName := f.String("event", "", "hook event")
	file := f.String("file", "", "source state JSON")
	from := f.String("from", "", "source ID")
	to := f.String("to", "", "target ID")
	limit := f.Int("limit", 5, "maximum cues per context section (1–20)")
	if err = f.Parse(args[1:]); err != nil {
		return err
	}
	text := strings.TrimSpace(strings.Join(f.Args(), " "))
	switch args[0] {
	case "event":
		data, err := io.ReadAll(io.LimitReader(in, core.MaxEventBytes+1))
		if err != nil {
			return err
		}
		e, err := core.DecodeEvent(data)
		if err != nil {
			return err
		}
		path, err := s.CaptureEvent(e)
		if err == nil {
			fmt.Fprintln(out, path)
		}
		return err
	case "context":
		o, err := s.Orient(*project, *limit)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(o)
		}
		fmt.Fprint(out, core.RenderOrientation(o))
		return nil
	case "checkpoint":
		path, err := s.Checkpoint(*project)
		if err == nil {
			fmt.Fprintln(out, path)
		}
		return err
	case "init":
		if err = s.Init(); err != nil {
			return err
		}
		p, err := s.Dashboard("")
		if err == nil {
			fmt.Fprintln(out, p)
		}
		return err
	case "extract":
		if *repo == "" {
			return errors.New("extract requires --repo PATH as the inventory root")
		}
		if *domain == "" {
			return errors.New("extract requires --domain home, work, or unknown")
		}
		if *source == "" || *source == "manual" {
			return errors.New("extract requires --source MACHINE_NAME")
		}
		notes, err := s.ExtractRepositories(*repo, *source, *domain)
		if err == nil {
			fmt.Fprint(out, core.InventorySummary(notes))
		}
		return err
	case "project":
		if *id == "" || *repo == "" || text == "" {
			return errors.New("project requires --id, --repo, and a title")
		}
		g, err := core.Inspect(*repo)
		if err != nil {
			return err
		}
		n := core.NewNote("project", text)
		n.ID = *id
		n.Repo = g.Root
		n.Status = "active"
		if *status != "" {
			n.Status = *status
		}
		if *domain != "" {
			n.Domain = *domain
		}
		n.Next = *next
		n.Body = "\n# " + text + "\n\nProject repository: `" + g.Root + "`.\n\nLinked work appears in backlinks and Thread's views.\n"
		p, err := s.New(n)
		if err == nil {
			fmt.Fprintln(out, p)
		}
		return err
	case "capture":
		body := text
		if *stdin {
			b, err := io.ReadAll(io.LimitReader(in, 1024*1024+1))
			if err != nil {
				return err
			}
			if len(b) > 1024*1024 {
				return errors.New("capture exceeds 1 MiB")
			}
			body = string(b)
			if text == "" {
				text = strings.SplitN(strings.TrimSpace(body), "\n", 2)[0]
			}
		}
		if strings.TrimSpace(body) == "" {
			return errors.New("capture needs text or --stdin")
		}
		if len([]rune(text)) > 160 {
			text = string([]rune(text)[:160]) + "…"
		}
		if *kind == "project" || *kind == "run" || *kind == "event" || *kind == "checkpoint" {
			return errors.New("use project/import/event/checkpoint for this kind")
		}
		n := core.NewNote(*kind, text)
		n.Source = *source
		n.Next = *next
		if *status != "" {
			n.Status = *status
		} else if *source != "manual" {
			n.Status = "suggested"
		}
		if *domain != "" {
			n.Domain = *domain
		}
		if *project != "" {
			p, err := s.Project(*project)
			if err != nil {
				return err
			}
			n.Project = s.Link(p)
			n.Domain = p.Domain
		}
		if *tags != "" {
			for _, tag := range strings.Split(*tags, ",") {
				if tag = strings.TrimSpace(tag); tag != "" {
					n.Tags = append(n.Tags, tag)
				}
			}
		}
		n.Body = "\n" + body + "\n"
		p, err := s.New(n)
		if err == nil {
			fmt.Fprintf(out, "Captured %s\n%s\n", n.ID, p)
		}
		return err
	case "set":
		supplied := map[string]bool{}
		f.Visit(func(x *flag.Flag) { supplied[x.Name] = true })
		if !supplied["status"] && !supplied["next"] && !supplied["domain"] {
			return errors.New("set requires a changed field")
		}
		return s.Update(*id, func(n *core.Note) error {
			if n.Kind == "run" {
				return errors.New("imported observations are immutable; create a linked action instead")
			}
			if supplied["status"] {
				n.Status = *status
			}
			if supplied["next"] {
				n.Next = *next
			}
			if supplied["domain"] {
				n.Domain = *domain
			}
			return nil
		})
	case "link":
		target, err := s.Find(*to)
		if err != nil {
			return err
		}
		return s.Update(*from, func(n *core.Note) error {
			link := s.Link(target)
			for _, v := range n.Related {
				if v == link {
					return nil
				}
			}
			n.Related = append(n.Related, link)
			return nil
		})
	case "show":
		n, err := s.Find(*id)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(n)
		}
		b, err := os.ReadFile(n.Path)
		if err == nil {
			_, err = out.Write(b)
		}
		return err
	case "status":
		if *domain != "" && *domain != "home" && *domain != "work" && *domain != "unknown" {
			return errors.New("invalid domain")
		}
		body, err := s.Overview(*domain)
		if err == nil {
			fmt.Fprint(out, body)
		}
		return err
	case "dashboard":
		p, err := s.Dashboard(*domain)
		if err == nil {
			fmt.Fprintf(out, "Updated Thread dashboard: %s\n", p)
		}
		return err
	case "snapshot":
		if *repo == "" {
			return errors.New("snapshot requires --repo PATH")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dest := filepath.Join(home, ".local", "state", "thread", "recovery", core.ID()+".zip")
		path, err := core.Snapshot(*repo, dest)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "Verified working-file snapshot:", path)
		fmt.Fprintln(out, "Excludes Git history/index, ignored files, unsaved buffers, symlinks, and submodules. Local copy only.")
		n := core.NewNote("memory", "Working-file recovery snapshot")
		n.Status = "done"
		n.Source = "thread-snapshot"
		n.Repo = *repo
		n.Extra = map[string]any{"recovery_path": path, "recovery_scope": "working-files", "recovery_verified_at": core.Now()}
		n.Body = "\nLocal working-file ZIP verified against its SHA-256 manifest: `" + path + "`.\n\nThis is not a full Git backup. Extract into a new directory when recovering; do not overwrite a live checkout.\n"
		p, e := s.ResolveProject(*project, *repo)
		if e == nil {
			n.Project = s.Link(p)
			n.Domain = p.Domain
		}
		_, err = s.New(n)
		return err
	case "verify":
		manifest, err := core.VerifySnapshot(*file)
		if err == nil {
			fmt.Fprintf(out, "Verified %d working files captured %s from %s\n", len(manifest.Files), manifest.Created, manifest.Git.Root)
		}
		return err
	case "hook":
		if *provider != "claude" && *provider != "codex" {
			return errors.New("hook supports --provider claude or codex")
		}
		if *eventName == "" {
			return errors.New("hook requires --event")
		}
		data, err := io.ReadAll(io.LimitReader(in, 4*1024*1024+1))
		if err != nil {
			return err
		}
		if len(data) > 4*1024*1024 {
			return errors.New("hook payload exceeds 4 MiB")
		}
		if *provider == "codex" {
			brief, err := s.HandleCodexHook(data, *eventName)
			if err == nil && brief != "" {
				fmt.Fprint(out, brief)
			}
			return err
		}
		e, err := core.DecodeHook(data)
		if err != nil {
			return err
		}
		if e.HookEventName == "" {
			e.HookEventName = *eventName
		}
		p, err := s.HandleClaudeHook(e)
		if err == nil {
			fmt.Fprintln(out, p)
		}
		return err
	case "organize":
		paths, err := s.Organize(*id, core.Claude)
		for _, p := range paths {
			fmt.Fprintln(out, p)
		}
		return err
	case "import":
		p, err := s.ImportRun(*provider, *file, *project)
		if err == nil {
			fmt.Fprintln(out, p)
		}
		return err
	case "resume":
		cwd := *repo
		if cwd == "" {
			cwd, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		p, err := s.ResolveProject(*project, cwd)
		if err != nil {
			return err
		}
		if *repo == "" && *project != "" {
			cwd = p.Repo
		}
		g, err := core.Inspect(cwd)
		if err != nil {
			return err
		}
		// Explicit project+repo must still identify the same repository.
		registered, err := core.Inspect(p.Repo)
		if err != nil {
			return err
		}
		if registered.Common != g.Common {
			return errors.New("selected repository does not match the project")
		}
		notes, err := s.Notes()
		if err != nil {
			return err
		}
		var items []core.Note
		for _, n := range notes {
			if core.SameLink(n.Project, s.Link(p)) && n.Status != "done" && n.Status != "archived" && n.Kind != "run" && n.Kind != "event" && n.Kind != "checkpoint" {
				items = append(items, n)
			}
		}
		var runs []core.Note
		for _, n := range core.LatestRuns(notes) {
			if core.SameLink(n.Project, s.Link(p)) {
				runs = append(runs, n)
			}
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(map[string]any{"project": p, "git": g, "items": items, "runs": runs})
		}
		fmt.Fprintf(out, "%s · %s · %s\n%s\nBranch: %s · HEAD: %s\n\n", p.Title, p.Domain, core.Machine(), g.Root, g.Branch, g.Head)
		if g.Changes != "" {
			fmt.Fprintln(out, "Local changes:\n"+g.Changes)
		} else {
			fmt.Fprintln(out, "Working copy is clean.")
		}
		fmt.Fprintln(out, "\nRecorded work (verify against current code):")
		for i, n := range items {
			if i == 8 {
				fmt.Fprintln(out, "More items are available in Obsidian.")
				break
			}
			fmt.Fprintf(out, "- %s [%s] — next: %s (updated %s)\n", n.Title, n.Status, empty(n.Next, "not recorded"), n.Updated)
		}
		for _, n := range runs {
			fmt.Fprintf(out, "- %s [%s], observed %s; source: %s\n", n.Title, n.Status, n.Updated, n.Extra["source_path"])
		}
		fmt.Fprintln(out, "\nRecovery:", g.Recovery)
		for _, n := range notes {
			if core.SameLink(n.Project, s.Link(p)) && n.Source == "thread-snapshot" {
				fmt.Fprintf(out, "Recorded working-file snapshot: %v (captured %s; verify before recovery, may predate current changes)\n", n.Extra["recovery_path"], n.Created)
				break
			}
		}
		fmt.Fprintln(out, "\nKnown worktrees:\n"+g.Worktrees)
		return nil
	default:
		return fmt.Errorf("unknown command %q; run thread help", args[0])
	}
}
func empty(a, b string) string {
	if a == "" {
		return b
	}
	return a
}
