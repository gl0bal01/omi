package cmd

import (
	"fmt"
	"io"

	"github.com/gl0bal01/omi/internal/api"
)

// buildChatRequest builds a UNIFY_CHAT_WITH_AI request with the layered legacy
// + nested settings shape the upstream accepts. Attachments are added by the
// caller post-construction.
func buildChatRequest(modelID, convUUID, prompt string, decision settingsDecision, mixed bool) *api.ChatRequest {
	return &api.ChatRequest{
		Type:           "UNIFY_CHAT_WITH_AI",
		Model:          modelID,
		ConversationID: convUUID,
		PromptObject: api.PromptObject{
			Prompt:         prompt,
			ConversationID: convUUID,
			WebSearch:      decision.WebSearch,
			NumOfSite:      decision.NumOfSite,
			MaxWord:        decision.MaxWord,
			IsMixed:        mixed,
			Settings: &api.PromptSettings{
				WebSearchSettings: &api.PromptWebSearchSettings{
					WebSearch: decision.WebSearch,
					NumOfSite: decision.NumOfSite,
					MaxWord:   decision.MaxWord,
				},
				HistorySettings: &api.PromptHistorySettings{
					IsMixed:           mixed,
					HistoryMixed:      mixed,
					HistoryMixedSnake: mixed,
				},
			},
		},
	}
}

// logEffective prints the canonical "omi: effective: ..." line when --verbose
// is set. Centralized so the format string lives in one place.
func logEffective(errOut io.Writer, decision settingsDecision, modelID string, f *chatFlags, hasAttachment bool) {
	if !runOpts.verbose {
		return
	}
	_, _ = fmt.Fprintf(errOut, "omi: effective: endpoint=/api/chat-with-ai stream=%t model=%s webSearch=%t numOfSite=%d maxWord=%d mixed=%t session=%t attachment=%t\n",
		!f.noStream, modelID, decision.WebSearch, decision.NumOfSite, decision.MaxWord, f.mixed, f.session != "", hasAttachment)
}

func logExplainDefaults(errOut io.Writer, decision settingsDecision) {
	if !runOpts.explainDefaults {
		return
	}
	_, _ = fmt.Fprintf(errOut, "omi: defaults: webSearch=%t (%s), numOfSite=%d (%s), maxWord=%d (%s)\n",
		decision.WebSearch, decision.WebSource, decision.NumOfSite, decision.NumOfSiteSource, decision.MaxWord, decision.MaxWordSource)
}
