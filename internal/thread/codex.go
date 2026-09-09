package thread

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// RunCodex wraps the Codex CLI with Thread lifecycle records. Codex keeps its
// normal terminal I/O; Thread records repository/session identity and process
// completion without attempting to parse the interactive transcript.
func (s *Store) RunCodex(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	for len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	sessionID := "codex-" + ID()
	start := ClaudeHookEvent{Provider: "codex", SessionID: sessionID, Cwd: cwd, HookEventName: "SessionStart"}
	if _, err = s.HandleClaudeHook(start); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = cwd, in, out, errOut
	runErr := cmd.Run()
	reason := "completed"
	message := "Codex exited successfully."
	if runErr != nil {
		reason = "process_error"
		message = "Codex exited: " + runErr.Error()
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			reason = "exit_" + strconv.Itoa(exitErr.ExitCode())
			message = fmt.Sprintf("Codex exited with status %d.", exitErr.ExitCode())
		}
	}
	end := ClaudeHookEvent{Provider: "codex", SessionID: sessionID, Cwd: filepath.Clean(cwd), HookEventName: "SessionEnd", Reason: reason, LastAssistantMessage: message}
	if _, endErr := s.HandleClaudeHook(end); runErr == nil {
		runErr = endErr
	}
	return runErr
}
