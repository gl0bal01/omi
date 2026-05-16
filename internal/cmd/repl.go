package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gl0bal01/omi/internal/api"
	"github.com/gl0bal01/omi/internal/closeutil"
	"github.com/gl0bal01/omi/internal/models"
	"github.com/gl0bal01/omi/internal/session"
	"golang.org/x/term"
)

// dispatchEmpty handles `omi` with no positional prompt: TTY → REPL loop,
// piped stdin → single read-all turn. Without -s, the auto-created
// conversation UUID is not persisted (orphan; lives on server only).
func dispatchEmpty(ctx context.Context, f *chatFlags, in *os.File, out, errOut io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, apiKey, err := loadAPIKey()
	if err != nil {
		return err
	}

	modelInput, taskErr := resolveModelInputWithTask(f.model, cfg.Model, f.task)
	if taskErr != nil {
		return &UsageError{Msg: taskErr.Error()}
	}
	modelID, defs, err := models.Resolve(modelInput, models.Capability(0))
	if err != nil {
		return &UsageError{Msg: err.Error()}
	}
	if defs.ConversationType == "code" {
		_, _ = fmt.Fprintf(errOut, "omi: hint: model %s works best with 'omi code'\n", modelInput)
	}
	decision := resolveSettings(f, defs)
	if decision.Notice {
		_, _ = fmt.Fprintf(errOut, "omi: note: enabling web search for %s (model default)\n", modelInput)
	}
	logExplainDefaults(errOut, decision)

	client := newAPIClient(apiKey)

	var (
		store    *session.Store
		convUUID string
	)
	if f.session != "" {
		store, err = session.New()
		if err != nil {
			return &RuntimeError{Msg: "omi: load sessions: " + err.Error()}
		}
		if uuid, ok := store.Get(f.session); ok {
			convUUID = uuid
		}
	}

	isTTY := term.IsTerminal(int(in.Fd()))
	if !isTTY {
		// Piped stdin: single-turn read-all, capped to bound memory.
		const maxStdinBytes = 4 * 1024 * 1024
		data, _ := io.ReadAll(io.LimitReader(in, maxStdinBytes+1))
		if len(data) > maxStdinBytes {
			return &UsageError{Msg: fmt.Sprintf("omi: stdin prompt exceeded %d bytes", maxStdinBytes)}
		}
		prompt := strings.TrimRight(string(data), "\r\n")
		if prompt == "" {
			return &UsageError{Msg: "omi: prompt required (positional arg or piped stdin)"}
		}
		if convUUID == "" && f.session != "" {
			convUUID, err = client.CreateConversation(ctx, "omi session: "+f.session, api.ConversationTypeUnify, modelID)
			if err != nil {
				return TranslateAPIError(err)
			}
			store.Set(f.session, convUUID)
			if err := store.Save(); err != nil {
				return &RuntimeError{Msg: "omi: save sessions: " + err.Error()}
			}
		}
		logEffective(errOut, decision, modelID, f, false)
		if err := runOneChatTurn(ctx, client, modelID, convUUID, prompt, decision, f, out, errOut); err != nil {
			return TranslateAPIError(err)
		}
		return nil
	}

	// TTY: REPL loop.
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for {
		_, _ = fmt.Fprint(out, "> ")
		if !scanner.Scan() {
			// EOF (Ctrl+D) — clean exit.
			_, _ = fmt.Fprintln(out)
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return nil
		}

		// Lazy-create conversation on first turn.
		if convUUID == "" {
			convUUID, err = client.CreateConversation(ctx, "omi session: "+sessionTitle(f.session), api.ConversationTypeUnify, modelID)
			if err != nil {
				_, _ = fmt.Fprintln(errOut, err.Error())
				continue
			}
			if f.session != "" {
				store.Set(f.session, convUUID)
				if err := store.Save(); err != nil {
					_, _ = fmt.Fprintln(errOut, err.Error())
				}
			}
		}

		logEffective(errOut, decision, modelID, f, false)
		if err := runOneChatTurn(ctx, client, modelID, convUUID, line, decision, f, out, errOut); err != nil {
			_, _ = fmt.Fprintln(errOut, err.Error())
		}
	}
}

// sessionTitle returns the conversation title segment for an unnamed REPL.
func sessionTitle(name string) string {
	if name == "" {
		return "repl"
	}
	return name
}

func runOneChatTurn(
	ctx context.Context,
	client *api.Client,
	modelID, convUUID, prompt string,
	decision settingsDecision,
	f *chatFlags,
	out, errOut io.Writer,
) error {
	body, err := client.Chat(ctx, buildChatRequest(modelID, convUUID, prompt, decision, f.mixed))
	if err != nil {
		return err
	}
	defer closeutil.Quiet(body)
	return consumeStream(body, f.noStream, out, errOut)
}
