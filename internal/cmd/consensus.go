package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/gl0bal01/omi/internal/api"
	"github.com/gl0bal01/omi/internal/closeutil"
	"github.com/gl0bal01/omi/internal/models"
	"github.com/spf13/cobra"
)

var defaultConsensusModels = []string{"mini", "claude", "gemini-pro"}

type consensusAnswer struct {
	Model  string
	APIID  string
	Answer string
}

func newConsensusCmd() *cobra.Command {
	var modelList string
	var synthModel string
	cmd := &cobra.Command{
		Use:           "consensus <prompt>",
		Short:         "Ask multiple chat models and synthesize a consensus",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, apiKey, err := loadAPIKey()
			if err != nil {
				return err
			}

			modelInputs, err := parseConsensusModels(modelList)
			if err != nil {
				return &UsageError{Msg: err.Error()}
			}
			synthInput := synthModel
			if synthInput == "" {
				synthInput = "best"
			}
			prompt := strings.Join(args, " ")
			client := newAPIClient(apiKey)
			result, err := runConsensus(cmd.Context(), client, modelInputs, synthInput, prompt)
			if err != nil {
				return err
			}
			printConsensus(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVarP(&modelList, "models", "m", strings.Join(defaultConsensusModels, ","), "comma-separated chat model aliases or IDs")
	cmd.Flags().StringVar(&synthModel, "synth-model", "best", "chat model alias or ID used to synthesize the final consensus")
	return cmd
}

type consensusResult struct {
	Answers []consensusAnswer
	Synth   consensusAnswer
}

func parseConsensusModels(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), defaultConsensusModels...), nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		m := strings.TrimSpace(part)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("omi: consensus requires at least two models")
	}
	return out, nil
}

// runConsensus fans the panel queries out concurrently then synthesizes once
// with the chosen synth model. Answer order matches modelInputs.
func runConsensus(ctx context.Context, client *api.Client, modelInputs []string, synthInput, prompt string) (consensusResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := consensusResult{Answers: make([]consensusAnswer, len(modelInputs))}

	apiIDs := make([]string, len(modelInputs))
	for i, input := range modelInputs {
		apiID, _, err := models.Resolve(input, 0)
		if err != nil {
			return result, &UsageError{Msg: err.Error()}
		}
		apiIDs[i] = apiID
	}
	synthAPIID, _, err := models.Resolve(synthInput, 0)
	if err != nil {
		return result, &UsageError{Msg: err.Error()}
	}

	panelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)
	for i := range modelInputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			answer, cerr := chatOnceForConsensus(panelCtx, client, apiIDs[i], prompt)
			if cerr != nil {
				errOnce.Do(func() {
					firstErr = TranslateAPIError(cerr)
					cancel()
				})
				return
			}
			result.Answers[i] = consensusAnswer{Model: modelInputs[i], APIID: apiIDs[i], Answer: strings.TrimSpace(answer)}
		}(i)
	}
	wg.Wait()
	if firstErr != nil {
		return result, firstErr
	}

	synthPrompt := buildConsensusPrompt(prompt, result.Answers)
	text, err := chatOnceForConsensus(ctx, client, synthAPIID, synthPrompt)
	if err != nil {
		return result, TranslateAPIError(err)
	}
	result.Synth = consensusAnswer{Model: synthInput, APIID: synthAPIID, Answer: strings.TrimSpace(text)}
	return result, nil
}

func chatOnceForConsensus(ctx context.Context, client *api.Client, modelID, prompt string) (string, error) {
	body, err := client.Chat(ctx, &api.ChatRequest{
		Type:  "UNIFY_CHAT_WITH_AI",
		Model: modelID,
		PromptObject: api.PromptObject{
			Prompt: prompt,
		},
	})
	if err != nil {
		return "", err
	}
	defer closeutil.Quiet(body)
	var out bytes.Buffer
	if err := consumeStream(body, true, &out, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

func buildConsensusPrompt(question string, answers []consensusAnswer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are synthesizing a consensus from %d independent model answers.\n", len(answers))
	fmt.Fprintf(&b, "Original question:\n%s\n\n", question)
	fmt.Fprintln(&b, "Model answers:")
	for _, a := range answers {
		fmt.Fprintf(&b, "\n[%s / %s]\n%s\n", a.Model, a.APIID, a.Answer)
	}
	fmt.Fprintln(&b, "\nReturn a concise report with exactly these sections:")
	fmt.Fprintln(&b, "Consensus:")
	fmt.Fprintln(&b, "Disagreements:")
	fmt.Fprintln(&b, "Recommendation:")
	return b.String()
}

func printConsensus(out io.Writer, result consensusResult) {
	_, _ = fmt.Fprintln(out, "Model answers:")
	for _, a := range result.Answers {
		_, _ = fmt.Fprintf(out, "\n[%s / %s]\n%s\n", a.Model, a.APIID, a.Answer)
	}
	_, _ = fmt.Fprintf(out, "\nConsensus synthesis [%s / %s]:\n%s\n", result.Synth.Model, result.Synth.APIID, result.Synth.Answer)
}
