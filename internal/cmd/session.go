package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gl0bal01/omi/internal/api"
	"github.com/gl0bal01/omi/internal/session"
	"github.com/spf13/cobra"
)

// errInvalidAPIKey is the in-process sentinel returned by clearOneSession /
// clearAllSessions when the server replies 401. The cobra wrapper translates
// it to "omi: invalid API key" + os.Exit(1). Tests assert on the sentinel
// directly without invoking os.Exit.
var errInvalidAPIKey = errors.New("omi: invalid API key")

// errSessionNotFound is returned by clearOneSession when the named session is
// missing from the local store. The cobra wrapper exits 2 in that case.
var errSessionNotFound = errors.New("omi: no such session")

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "session",
		Short:         "Manage named conversations (list, clear)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newSessionListCmd())
	cmd.AddCommand(newSessionClearCmd())
	return cmd
}

func newSessionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List saved sessions",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := session.New()
			if err != nil {
				return fmt.Errorf("omi: load sessions: %w", err)
			}
			out := cmd.OutOrStdout()
			list := s.List()
			if len(list) == 0 {
				_, _ = fmt.Fprintln(out, "(no sessions)")
				return nil
			}
			names := make([]string, 0, len(list))
			for k := range list {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, n := range names {
				_, _ = fmt.Fprintf(out, "%s\t%s\n", n, list[n])
			}
			return nil
		},
	}
}

func newSessionClearCmd() *cobra.Command {
	var (
		all bool
		yes bool
	)
	cmd := &cobra.Command{
		Use:           "clear [name]",
		Short:         "Delete one or all sessions (server-side + local)",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, apiKey, err := loadAPIKey()
			if err != nil {
				return err
			}

			store, err := session.New()
			if err != nil {
				return &RuntimeError{Msg: "omi: load sessions: " + err.Error()}
			}
			client := newAPIClient(apiKey)

			if all {
				if err := clearAllSessions(cmd.Context(), client, store, yes, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
					return translateSessionExitError(err)
				}
				return nil
			}

			if len(args) == 0 {
				return &UsageError{Msg: "omi: session clear requires a name (or --all)"}
			}
			name := args[0]
			if err := clearOneSession(cmd.Context(), client, store, name, cmd.ErrOrStderr()); err != nil {
				return translateSessionExitError(err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "delete all sessions")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt for --all")
	return cmd
}

// clearOneSession deletes a single session. Behavior matches plan §3 Phase 5:
//   - missing name → returns errSessionNotFound (cobra wrapper exits 2).
//   - 401 from DELETE → returns errInvalidAPIKey, local entry preserved.
//   - nil (incl. 404) → remove local entry, save, return nil silently.
//   - other error → warn on stderr, remove local entry, save, return nil.
func clearOneSession(ctx context.Context, client *api.Client, store *session.Store, name string, errOut io.Writer) error {
	uuid, ok := store.Get(name)
	if !ok {
		_, _ = fmt.Fprintf(errOut, "omi: no such session: %s\n", name)
		return errSessionNotFound
	}

	err := client.DeleteConversation(ctx, uuid)
	if err != nil {
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			_, _ = fmt.Fprintln(errOut, "omi: invalid API key")
			return errInvalidAPIKey
		}
		_, _ = fmt.Fprintf(errOut, "omi: warning: server-side delete failed for session '%s': %v; removing local entry anyway\n", name, err)
	}
	store.Delete(name)
	if err := store.Save(); err != nil {
		return fmt.Errorf("omi: save sessions: %w", err)
	}
	return nil
}

// clearAllSessions iterates stored sessions in sorted order, sending one DELETE
// per UUID. 401 mid-loop aborts and leaves the local file untouched. Other
// errors warn + continue. Final wipe occurs only on full traversal.
func clearAllSessions(ctx context.Context, client *api.Client, store *session.Store, skipPrompt bool, in io.Reader, out, errOut io.Writer) error {
	list := store.List()
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "(no sessions)")
		return nil
	}

	if !skipPrompt {
		_, _ = fmt.Fprintf(errOut, "Delete all %d sessions? [y/N]: ", len(list))
		scanner := bufio.NewScanner(in)
		if !scanner.Scan() {
			_, _ = fmt.Fprintln(errOut, "aborted")
			return nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans != "y" && ans != "yes" {
			_, _ = fmt.Fprintln(errOut, "aborted")
			return nil
		}
	}

	names := make([]string, 0, len(list))
	for k := range list {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		uuid := list[name]
		err := client.DeleteConversation(ctx, uuid)
		if err == nil {
			continue
		}
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			_, _ = fmt.Fprintln(errOut, "omi: invalid API key")
			return errInvalidAPIKey
		}
		_, _ = fmt.Fprintf(errOut, "omi: warning: server-side delete failed for session '%s': %v; removing local entry anyway\n", name, err)
	}

	store.Clear()
	if err := store.Save(); err != nil {
		return fmt.Errorf("omi: save sessions: %w", err)
	}
	return nil
}

// translateSessionExitError converts in-process sentinels into typed errors
// that main.go maps to the proper shell exit codes (RuntimeError → 1,
// UsageError → 2). Returns the error unchanged when no sentinel matches.
func translateSessionExitError(err error) error {
	if errors.Is(err, errInvalidAPIKey) {
		return &RuntimeError{Msg: "omi: invalid API key"}
	}
	if errors.Is(err, errSessionNotFound) {
		return &UsageError{Msg: "omi: no such session"}
	}
	return err
}
