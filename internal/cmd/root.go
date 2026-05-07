package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gl0bal01/omi/internal/api"
	"github.com/gl0bal01/omi/internal/closeutil"
	"github.com/gl0bal01/omi/internal/models"
	"github.com/gl0bal01/omi/internal/session"
	"github.com/gl0bal01/omi/internal/stream"
	"github.com/spf13/cobra"
)

const defaultChatModel = "gpt-4o-mini"

type chatFlags struct {
	model    string
	task     string
	session  string
	file     string
	web      bool
	noWeb    bool
	mixed    bool
	noStream bool
	numSites int
	maxWords int
	// Set by cobra Changed() at runtime to distinguish "user passed" vs default.
	numSitesSet bool
	maxWordsSet bool
}

func newRootCmd(version string) *cobra.Command {
	var f chatFlags
	cmd := &cobra.Command{
		Use:           "omi [prompt]",
		Short:         "1min.ai CLI — chat, consensus, code, vision, transcription",
		Example:       "  omi \"explain Go interfaces\"\n  omi consensus \"Should we ship this?\"\n  omi consensus -m mini,claude,gemini-pro --synth-model best \"Compare these options\"",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f.numSitesSet = cmd.Flags().Changed("num-sites")
			f.maxWordsSet = cmd.Flags().Changed("max-words")
			return runChat(cmd.Context(), args, &f)
		},
	}

	cmd.Flags().StringVarP(&f.model, "model", "m", "", "chat model alias or id")
	cmd.Flags().StringVar(&f.task, "task", "", "task preset for model selection (chat, code, vision, research)")
	cmd.Flags().StringVarP(&f.session, "session", "s", "", "named session (persists conversation UUID)")
	cmd.Flags().StringVarP(&f.file, "file", "f", "", "image (jpg/png/...) or document (pdf/txt/md/docx) attachment")
	cmd.Flags().BoolVarP(&f.web, "web", "w", false, "enable web search")
	cmd.Flags().BoolVar(&f.noWeb, "no-web", false, "disable web search")
	cmd.Flags().BoolVarP(&f.mixed, "mixed", "M", false, "isMixed flag on promptObject")
	cmd.Flags().BoolVar(&f.noStream, "no-stream", false, "buffer the response and print at the end")
	cmd.Flags().IntVarP(&f.numSites, "num-sites", "n", 0, "numOfSite for web search")
	cmd.Flags().IntVar(&f.maxWords, "max-words", 0, "maxWord cap on response")
	cmd.PersistentFlags().BoolVar(&runOpts.verbose, "verbose", false, "print effective request settings")
	cmd.PersistentFlags().BoolVar(&runOpts.explainDefaults, "explain-defaults", false, "explain how final defaults were selected")
	cmd.PersistentFlags().BoolVar(&runOpts.debugHTTP, "debug-http", false, "print redacted HTTP request/response debug logs")
	cmd.PersistentFlags().StringVar(&runOpts.apiKey, "api-key", "", "API key (overrides OMI_API_KEY env and config file)")
	cmd.PersistentFlags().DurationVar(&runOpts.timeout, "timeout", 0, "request timeout for non-streaming calls (default 60s)")

	_ = cmd.RegisterFlagCompletionFunc("model", chatModelCompletion)
	_ = cmd.RegisterFlagCompletionFunc("session", sessionCompletion)

	cmd.AddCommand(newUploadCmd())
	cmd.AddCommand(newTranscribeCmd())
	cmd.AddCommand(newQuickstartCmd())
	cmd.AddCommand(newCodeCmd())
	cmd.AddCommand(newConsensusCmd())
	cmd.AddCommand(newModelsCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newSessionCmd())
	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newCompletionCmd(cmd))

	return cmd
}

// chatModelCompletion lists chat-eligible aliases (excludes code-only and
// vision-only entries) for `-m/--model` on the root chat command.
func chatModelCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	out := []string{}
	for _, e := range models.Entries() {
		if e.Caps == models.CapCode || e.Caps == models.CapVision {
			continue
		}
		if strings.HasPrefix(e.Alias, toComplete) {
			out = append(out, e.Alias)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// sessionCompletion lists saved session names from the local store.
func sessionCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	store, err := session.New()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := []string{}
	for name := range store.List() {
		if strings.HasPrefix(name, toComplete) {
			out = append(out, name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func Execute(version string) error {
	return ExecuteContext(context.Background(), version)
}

// ExecuteContext runs the CLI with the given context so SIGINT/SIGTERM cancel
// in-flight HTTP and SSE work.
func ExecuteContext(ctx context.Context, version string) error {
	return newRootCmd(version).ExecuteContext(ctx)
}

func runChat(ctx context.Context, args []string, f *chatFlags) error {
	if ctx == nil {
		ctx = context.Background()
	}
	prompt := strings.Join(args, " ")
	if prompt == "" {
		// TTY → REPL; piped stdin → single-turn read-all.
		return dispatchEmpty(ctx, f, os.Stdin, os.Stdout, os.Stderr)
	}

	cfg, apiKey, err := loadAPIKey()
	if err != nil {
		return err
	}

	modelInput, taskErr := resolveModelInputWithTask(f.model, cfg.Model, f.task)
	if taskErr != nil {
		return &UsageError{Msg: taskErr.Error()}
	}

	var fileKindResolved fileKind
	var hasFile bool
	if f.file != "" {
		fk, derr := detectFile(f.file)
		if derr != nil {
			return &UsageError{Msg: derr.Error()}
		}
		if fk == fileAudio {
			return &UsageError{Msg: "omi: -f does not support audio (use 'omi transcribe')"}
		}
		fileKindResolved = fk
		hasFile = true
	}

	required := models.Capability(0)
	if hasFile && fileKindResolved == fileImage {
		required = models.CapVision
	}

	modelID, defs, err := models.Resolve(modelInput, required)
	if err != nil {
		return &UsageError{Msg: err.Error()}
	}

	if defs.ConversationType == "code" {
		_, _ = fmt.Fprintf(os.Stderr, "omi: hint: model %s works best with 'omi code'\n", modelInput)
	}

	decision := resolveSettings(f, defs)
	if decision.Notice {
		_, _ = fmt.Fprintf(os.Stderr, "omi: note: enabling web search for %s (model default)\n", modelInput)
	}
	logExplainDefaults(os.Stderr, decision)

	client := newAPIClient(apiKey)

	var convUUID string
	if f.session != "" {
		store, sErr := session.New()
		if sErr != nil {
			return &RuntimeError{Msg: "omi: load sessions: " + sErr.Error()}
		}
		if uuid, ok := store.Get(f.session); ok {
			convUUID = uuid
		} else {
			uuid, cErr := client.CreateConversation(ctx, "omi session: "+f.session, api.ConversationTypeUnify, modelID)
			if cErr != nil {
				return TranslateAPIError(cErr)
			}
			store.Set(f.session, uuid)
			if err := store.Save(); err != nil {
				return &RuntimeError{Msg: "omi: save sessions: " + err.Error()}
			}
			convUUID = uuid
		}
	}

	req := buildChatRequest(modelID, convUUID, prompt, decision, f.mixed)

	if hasFile {
		assetPath, err := client.UploadAsset(ctx, f.file)
		if err != nil {
			return TranslateAPIError(err)
		}
		switch fileKindResolved {
		case fileImage:
			req.ImageList = []string{assetPath}
			req.PromptObject.Attachments = &api.PromptAttachments{Images: []string{assetPath}}
		case fileDoc:
			req.Files = []string{assetPath}
			req.PromptObject.Attachments = &api.PromptAttachments{Files: []string{assetPath}}
		}
	}
	logEffective(os.Stderr, decision, modelID, f, hasFile)

	body, err := client.Chat(ctx, req)
	if err != nil {
		return TranslateAPIError(err)
	}
	defer closeutil.Quiet(body)

	return consumeStream(body, f.noStream, os.Stdout, os.Stderr)
}

// settingsDecision holds final webSearch/numOfSite/maxWord plus the source
// label used by --explain-defaults. Notice=true ONLY when defs.WebSearch
// caused an auto-enable that the caller must announce on stderr.
//
// Precedence:
//   - --no-web → webSearch=false, no notice.
//   - -w/--web → webSearch=true, no notice.
//   - else if defs.WebSearch=true → webSearch=true + notice.
//   - else → webSearch=false.
//
// numOfSite/maxWord: explicit user flag (Changed()) wins; else use defs;
// else 0 (server default).
type settingsDecision struct {
	WebSearch       bool
	NumOfSite       int
	MaxWord         int
	Notice          bool
	WebSource       string
	NumOfSiteSource string
	MaxWordSource   string
}

func resolveSettings(f *chatFlags, defs models.ModelDefaults) settingsDecision {
	var d settingsDecision
	switch {
	case f.noWeb:
		d.WebSearch = false
		d.WebSource = "--no-web"
	case f.web:
		d.WebSearch = true
		d.WebSource = "--web"
	case defs.WebSearch != nil && *defs.WebSearch:
		d.WebSearch = true
		d.Notice = true
		d.WebSource = "model default"
	default:
		d.WebSearch = false
		d.WebSource = "implicit default"
	}

	switch {
	case f.numSitesSet:
		d.NumOfSite = f.numSites
		d.NumOfSiteSource = "--num-sites"
	case defs.NumOfSite != nil:
		d.NumOfSite = *defs.NumOfSite
		d.NumOfSiteSource = "model default"
	default:
		d.NumOfSite = 0
		d.NumOfSiteSource = "server default"
	}

	switch {
	case f.maxWordsSet:
		d.MaxWord = f.maxWords
		d.MaxWordSource = "--max-words"
	case defs.MaxWord != nil:
		d.MaxWord = *defs.MaxWord
		d.MaxWordSource = "model default"
	default:
		d.MaxWord = 0
		d.MaxWordSource = "server default"
	}
	return d
}

// resolveModelInput returns the user-facing model identifier (alias OR raw
// id) BEFORE registry resolution. The empty string indicates "no choice"
// and the caller falls back to defaultChatModel.
func resolveModelInput(flag, fromCfg string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("OMI_MODEL"); env != "" {
		return env
	}
	if fromCfg != "" {
		return fromCfg
	}
	return defaultChatModel
}

func resolveModelInputWithTask(flag, fromCfg, task string) (string, error) {
	if flag != "" || os.Getenv("OMI_MODEL") != "" || fromCfg != "" {
		return resolveModelInput(flag, fromCfg), nil
	}
	if task == "" {
		return resolveModelInput(flag, fromCfg), nil
	}
	switch task {
	case "chat":
		return "mini", nil
	case "code":
		return "gpt-5.1-codex", nil
	case "vision":
		return "qwen3-vl-plus", nil
	case "research":
		return "deep-research", nil
	case "transcribe":
		return "", fmt.Errorf("omi: --task transcribe is not supported on chat; use 'omi transcribe'")
	default:
		return "", fmt.Errorf("omi: invalid --task '%s' (use: chat, code, vision, research)", task)
	}
}

// maxNoStreamBytes caps buffered output under --no-stream to bound memory if
// the upstream pumps an unbounded response. Excess is dropped with one stderr
// warning.
const maxNoStreamBytes = 16 * 1024 * 1024

// consumeStream drives the SSE parser and writes content to out. On --no-stream
// it buffers content and prints once at the end (capped at maxNoStreamBytes).
func consumeStream(r io.Reader, noStream bool, out io.Writer, errOut io.Writer) error {
	ch := make(chan stream.Event, 16)
	errCh := make(chan error, 1)
	go func() { errCh <- stream.Parse(r, ch) }()

	var buf bytes.Buffer
	var lastChunk string
	var streamErr error
	var truncated bool

	for ev := range ch {
		switch ev.Type {
		case stream.EventContent:
			chunk := extractContentChunk(ev.Data)
			chunk = sanitizeForTerminalText(chunk)
			if noStream {
				if buf.Len()+len(chunk) > maxNoStreamBytes {
					if !truncated {
						remain := maxNoStreamBytes - buf.Len()
						if remain > 0 {
							buf.WriteString(chunk[:remain])
						}
						_, _ = fmt.Fprintf(errOut, "omi: warning: response exceeded %d bytes; truncated\n", maxNoStreamBytes)
						truncated = true
					}
					continue
				}
				buf.WriteString(chunk)
			} else {
				_, _ = io.WriteString(out, chunk)
				if f, ok := out.(interface{ Sync() error }); ok {
					_ = f.Sync()
				}
				lastChunk = chunk
			}
		case stream.EventError:
			streamErr = &RuntimeError{Msg: "omi: " + extractErrorMessage(ev.Data)}
		case stream.EventDone, stream.EventResult:
		}
	}
	if perr := <-errCh; perr != nil && streamErr == nil {
		return &RuntimeError{Msg: "omi: stream parse: " + perr.Error()}
	}
	if streamErr != nil {
		return streamErr
	}

	if noStream {
		_, _ = out.Write(buf.Bytes())
		_, _ = io.WriteString(out, "\n")
		return nil
	}
	if !strings.HasSuffix(lastChunk, "\n") {
		_, _ = io.WriteString(out, "\n")
	}
	return nil
}

func extractContentChunk(data string) string {
	data = strings.TrimSpace(data)
	if data == "" {
		return ""
	}
	if !strings.HasPrefix(data, "{") {
		return sanitizeForTerminalText(data)
	}
	var payload struct {
		Content *string `json:"content"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return data
	}
	if payload.Content != nil {
		return *payload.Content
	}
	return data
}

func extractErrorMessage(data string) string {
	data = strings.TrimSpace(data)
	if data == "" {
		return "stream error"
	}
	if !strings.HasPrefix(data, "{") {
		return data
	}
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return sanitizeForTerminalText(data)
	}
	if payload.Message != "" {
		return sanitizeForTerminalText(payload.Message)
	}
	if payload.Error != "" {
		return sanitizeForTerminalText(payload.Error)
	}
	return sanitizeForTerminalText(data)
}

func sanitizeForTerminalText(s string) string {
	return sanitizeForTerminalWithAllowedControls(s, "\n\r\t")
}
