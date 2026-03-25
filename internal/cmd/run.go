package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"charm.land/log/v2"
	"github.com/charmbracelet/crush/internal/event"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Aliases: []string{"r"},
	Use:     "run [prompt...]",
	Short:   "Run a single non-interactive prompt",
	Long: `Run a single prompt in non-interactive mode and exit.
The prompt can be provided as arguments or piped from stdin.`,
	Example: `
# Run a simple prompt
crusher run "Guess my 5 favorite Pokémon"

# Pipe input from stdin
curl https://charm.land | crusher run "Summarize this website"

# Read from a file
crusher run "What is this code doing?" <<< prrr.go

# Redirect output to a file
crusher run "Generate a hot README for this project" > MY_HOT_README.md

# Run in quiet mode (hide the spinner)
crusher run --quiet "Generate a README for this project"

# Run in verbose mode (show logs)
crusher run --verbose "Generate a README for this project"

# AI DEBUG MODE: Full transparency, X-ray vision into all internal operations
# Shows: audit trails, circuit breaker decisions, ghost compact, token usage
crusher run --ai-debug "Your prompt here"

# Continue a previous session
crusher run --session {session-id} "Follow up on your last response"

# Continue the most recent session
crusher run --continue "Follow up on your last response"

  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			quiet, _      = cmd.Flags().GetBool("quiet")
			verbose, _    = cmd.Flags().GetBool("verbose")
			aiDebug, _    = cmd.Flags().GetBool("ai-debug")
			verbosity, _  = cmd.Flags().GetString("verbosity")
			largeModel, _ = cmd.Flags().GetString("model")
			smallModel, _ = cmd.Flags().GetString("small-model")
			sessionID, _  = cmd.Flags().GetString("session")
			useLast, _    = cmd.Flags().GetBool("continue")
		)

		// Cancel on SIGINT or SIGTERM.
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
		defer cancel()

		app, err := setupApp(cmd)
		if err != nil {
			return err
		}
		defer app.Shutdown()

		if sessionID != "" {
			sess, err := resolveSessionID(ctx, app.Sessions, sessionID)
			if err != nil {
				return err
			}
			sessionID = sess.ID
		}

		if !app.Config().IsConfigured() {
			return fmt.Errorf("no providers configured - please run 'crush' to set up a provider interactively")
		}

		if verbose {
			slog.SetDefault(slog.New(log.New(os.Stderr)))
		}

		prompt := strings.Join(args, " ")

		prompt, err = MaybePrependStdin(prompt)
		if err != nil {
			slog.Error("Failed to read from stdin", "error", err)
			return err
		}

		if prompt == "" {
			return fmt.Errorf("no prompt provided")
		}

		event.SetNonInteractive(true)
		event.AppInitialized()

		switch {
		case sessionID != "":
			event.SetContinueBySessionID(true)
		case useLast:
			event.SetContinueLastSession(true)
		}

		if aiDebug {
			return app.RunNonInteractiveDebug(ctx, os.Stdout, prompt, largeModel, smallModel, verbosity, sessionID, useLast)
		}
		return app.RunNonInteractive(ctx, os.Stdout, prompt, largeModel, smallModel, quiet || verbose, sessionID, useLast)
	},
}

func init() {
	runCmd.Flags().BoolP("quiet", "q", false, "Hide spinner")
	runCmd.Flags().BoolP("verbose", "v", false, "Show logs")
	runCmd.Flags().Bool("ai-debug", false, "AI Debug mode: pure CLI with full transparency, X-ray vision into all internal operations")
	runCmd.Flags().String("verbosity", "normal", "Verbosity level for ai-debug mode: minimal, normal, full, tokens")
	runCmd.Flags().StringP("model", "m", "", "Model to use. Accepts 'model' or 'provider/model' to disambiguate models with the same name across providers")
	runCmd.Flags().String("small-model", "", "Small model to use. If not provided, uses the default small model for the provider")
	runCmd.Flags().StringP("session", "s", "", "Continue a previous session by ID")
	runCmd.Flags().BoolP("continue", "C", false, "Continue the most recent session")
	runCmd.MarkFlagsMutuallyExclusive("session", "continue")
}
