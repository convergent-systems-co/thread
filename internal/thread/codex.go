package thread

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	project, err := s.ResolveProject("", cwd)
	if err != nil {
		return err
	}
	start := Event{Schema: 1, ID: sessionID + "-start", Source: "codex",
		SessionID: sessionID, Repo: cwd, ProjectID: project.ID, Machine: Machine(),
		Type: "session.started", OccurredAt: Now()}
	if _, err = s.CaptureEvent(start); err != nil {
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
	end := start
	end.ID, end.Type, end.OccurredAt = sessionID+"-end", "session.stopped", Now()
	end.Text, end.Data = message, map[string]any{"reason": reason}
	_, endErr := s.CaptureEvent(end)
	return errors.Join(runErr, endErr)
}
