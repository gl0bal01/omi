package cmd

import (
	"io"

	"github.com/spf13/cobra"
)

const quickstartText = `omi quickstart

General chat
  Fast/cheap: ./bin/omi -m mini "..."
  Best quality: ./bin/omi -m best "..."
  Auto research: ./bin/omi -m deep-research "..."

Code generation
  Default: ./bin/omi code "Implement ..."
  Fast alt: ./bin/omi code --code-model qwen-code-fast "..."
  Task preset: ./bin/omi --task code "Write tests for ..."

3-model consensus (omi client-side feature)
  Default panel: ./bin/omi consensus "Should we ship this?"
  Custom panel: ./bin/omi consensus -m mini,claude,gemini-pro --synth-model best "..."

Vision / files
  Image QA: ./bin/omi --task vision -f image.png "What is this?"
  Document QA: ./bin/omi -f doc.pdf "Summarize"

Sessions and mixed history
  Start/reuse a named session: ./bin/omi -s research "Remember this context"
  Continue that session: ./bin/omi -s research "Use the context from earlier"
  Mixed-history context: ./bin/omi --mixed "Compare this with prior context"
  Persistent mixed session: ./bin/omi -s research --mixed "Continue with mixed history"
  List sessions: ./bin/omi session list
  Clear one session: ./bin/omi session clear research

Transcription
  Default ASR: ./bin/omi transcribe audio.mp3
  Telephony: ./bin/omi transcribe -m asr-telephony call.wav
  Diarization: ./bin/omi transcribe -m asr-diarize meeting.mp3
  List options: ./bin/omi transcribe models

Inspect and debug
  Model catalog: ./bin/omi models all
  Explain one model: ./bin/omi models explain sonar
  Health check: ./bin/omi doctor --live
  Effective settings: ./bin/omi --verbose --explain-defaults "..."
  HTTP debug: ./bin/omi --debug-http --no-stream "..."
`

func newQuickstartCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "quickstart",
		Short:         "Show recommended model presets by use case",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), quickstartText)
			return err
		},
	}
}
