package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/event"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Aliases: []string{"c"},
	Use:     "chat [prompt...]",
	Short:   "Interactive chat mode with full transparency",
	Long: `Interactive chat mode with X-ray vision into all internal operations.
Reads prompts from stdin or arguments and maintains a session for context.
Press Ctrl+C or Ctrl+D to exit.`,
	Example: `
# Start interactive chat
crush chat

# Start with AI debug mode (X-ray vision)
crush chat --ai-debug

# Single prompt in chat mode
crush chat "Hello, how are you?"

# Continue a previous session
crush chat --session {session-id}

# Continue the most recent session
crush chat --continue

# Pipe input to chat
echo "Hello" | crush chat --ai-debug
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			aiDebug, _   = cmd.Flags().GetBool("ai-debug")
			sessionID, _  = cmd.Flags().GetString("session")
			useLast, _    = cmd.Flags().GetBool("continue")
		)

		// Set up context with cancellation on SIGINT/SIGTERM
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			slog.Info("chat: received interrupt, shutting down gracefully")
			cancel()
		}()

		// Set up app
		appInstance, err := setupApp(cmd)
		if err != nil {
			return err
		}
		defer appInstance.Shutdown()

		// Wait for MCP initialization
		if err := mcp.WaitForInit(ctx); err != nil {
			return fmt.Errorf("failed to wait for MCP initialization: %w", err)
		}
		appInstance.AgentCoordinator.UpdateModels(ctx)

		// Auto-approve permissions
		appInstance.Permissions.AutoApproveSession("chat")

		// Resolve or create session
		sess, err := appInstance.ResolveSession(ctx, sessionID, useLast)
		if err != nil {
			return fmt.Errorf("failed to resolve session: %w", err)
		}
		sessionID = sess.ID

		// Create debugger if in AI debug mode
		var debugger *agent.AIDebugger
		if aiDebug {
			debugger = agent.NewAIDebugger(agent.DefaultDebugConfig())
			agent.ClearAuditTrail()
		}

		// Single flag to ensure welcome prints only once
		var welcomePrinted bool
		var welcomeMu sync.Mutex

		printWelcome := func() {
			welcomeMu.Lock()
			if welcomePrinted {
				welcomeMu.Unlock()
				return
			}
			welcomePrinted = true
			welcomeMu.Unlock()

			if aiDebug && debugger != nil {
				debugger.Header("CHAT SESSION START (AI DEBUG MODE)")
				debugger.KV("SessionID", sessionID)
				debugger.KV("Mode", "X-RAY VISION")
				model := appInstance.AgentCoordinator.Model()
				if model.Model != nil {
					debugger.KV("Model", model.ModelCfg.Model)
					debugger.KV("Provider", model.ModelCfg.Provider)
					debugger.KV("ContextWindow", model.CatwalkCfg.ContextWindow)
				}
				cfg := appInstance.Config()
				debugger.KV("CircuitBreakerEnabled", cfg.Options.EnableCircuitBreaker)
				debugger.KV("GhostCountEnabled", cfg.Options.EnableGhostCount)
				debugger.KV("Exit", "Ctrl+C or Ctrl+D")
				fmt.Fprint(os.Stderr, debugger.String())
				debugger.Reset()
			} else {
				fmt.Fprint(os.Stderr, "Crush Chat (type 'exit' or Ctrl+C to quit)\n\n")
			}
		}

		printPrompt := func() {
			if !aiDebug {
				fmt.Fprint(os.Stderr, "> ")
			}
		}

		event.SetNonInteractive(true)
		event.AppInitialized()

		// If args provided, treat as single prompt
		if len(args) > 0 {
			prompt := strings.Join(args, " ")
			return runChatPrompt(ctx, appInstance, sessionID, prompt, debugger, aiDebug)
		}

		// Interactive stdin loop
		printWelcome()

		stdin := bufio.NewReader(os.Stdin)
		for {
			printPrompt()

			prompt, err := stdin.ReadString('\n')
			if err == io.EOF {
				if aiDebug && debugger != nil {
					debugger.Header("SESSION END (EOF)")
					debugger.PrintAllAuditTrail(agent.GetAuditTrail())
					debugger.PrintSummary(agent.GetAuditTrail(), agent.GetCircuitBreakerRetryCount(sessionID))
					fmt.Fprint(os.Stderr, debugger.String())
				}
				return nil
			}
			if err != nil {
				return fmt.Errorf("read error: %w", err)
			}

			prompt = strings.TrimSpace(prompt)
			if prompt == "" {
				continue
			}

			if prompt == "exit" || prompt == "quit" || prompt == "q" {
				if aiDebug && debugger != nil {
					debugger.Header("SESSION END (USER EXIT)")
					debugger.PrintAllAuditTrail(agent.GetAuditTrail())
					debugger.PrintSummary(agent.GetAuditTrail(), agent.GetCircuitBreakerRetryCount(sessionID))
					fmt.Fprint(os.Stderr, debugger.String())
				}
				return nil
			}

			if err := runChatPrompt(ctx, appInstance, sessionID, prompt, debugger, aiDebug); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				slog.Error("chat: prompt error", "error", err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	},
}

func init() {
	chatCmd.Flags().Bool("ai-debug", false, "AI Debug mode: full transparency, X-ray vision into all internal operations")
	chatCmd.Flags().StringP("session", "s", "", "Continue a previous session by ID")
	chatCmd.Flags().BoolP("continue", "C", false, "Continue the most recent session")
	chatCmd.MarkFlagsMutuallyExclusive("session", "continue")
}

func runChatPrompt(ctx context.Context, appInstance *app.App, sessionID, prompt string, debugger *agent.AIDebugger, aiDebug bool) error {
	if aiDebug && debugger != nil {
		debugger.Header("PROMPT")
		debugger.KV("Prompt", truncateString(prompt, 200))
		fmt.Fprint(os.Stderr, debugger.String())
		debugger.Reset()
	}

	// Channel for results
	type response struct {
		result *fantasy.AgentResult
		err    error
	}
	done := make(chan response, 1)

	startTime := time.Now()

	// Think callback for streaming model reasoning
	thinkCallback := func(text string) {
		if aiDebug && debugger != nil {
			debugger.SubHeader("THINK")
			debugger.KV("Reasoning", text)
			fmt.Fprint(os.Stderr, debugger.String())
			debugger.Reset()
		}
	}

	go func() {
		result, err := appInstance.AgentCoordinator.Run(ctx, sessionID, prompt, thinkCallback)
		if err != nil {
			done <- response{err: fmt.Errorf("agent failed: %w", err)}
			return
		}
		done <- response{result: result}
	}()

	// Subscribe to message events
	messageEvents := appInstance.Messages.Subscribe(ctx)
	messageReadBytes := make(map[string]int)
	toolCallCount := 0

	for {
		select {
		case <-ctx.Done():
			if aiDebug && debugger != nil {
				debugger.Header("PROMPT CANCELLED")
				debugger.PrintAllAuditTrail(agent.GetAuditTrail())
				fmt.Fprint(os.Stderr, debugger.String())
				debugger.Reset()
			}
			return ctx.Err()

		case result := <-done:
			duration := time.Since(startTime)

			if aiDebug && debugger != nil {
				auditEntries := agent.GetAuditTrail()
				debugger.Header("RESPONSE COMPLETE")
				debugger.KV("Duration", duration)
				debugger.KV("ToolCalls", toolCallCount)
				debugger.KV("AuditEntries", len(auditEntries))
				debugger.KV("RecoveryAttempts", agent.GetCircuitBreakerRetryCount(sessionID))

				// Show last few audit entries if any
				if len(auditEntries) > 0 {
					debugger.SubHeader("RECENT AUDIT ENTRIES")
					start := 0
					if len(auditEntries) > 5 {
						start = len(auditEntries) - 5
					}
					for i, entry := range auditEntries[start:] {
						idx := start + i + 1
						debugger.KV(fmt.Sprintf("[%d] %s", idx, entry.Timestamp.Format("15:04:05")),
							fmt.Sprintf("%s | %s | %v", entry.Action, agent.StrategyName(entry.Strategy), entry.Success))
					}
				}
				fmt.Fprint(os.Stderr, debugger.String())
				debugger.Reset()
			} else {
				// Simple non-debug output
				fmt.Fprint(os.Stderr, "\n")
			}

			if result.err != nil {
				return result.err
			}
			return nil

		case event := <-messageEvents:
			msg := event.Payload

			if msg.SessionID != sessionID {
				continue
			}

			if msg.Role == message.Assistant && len(msg.Parts) > 0 {
				content := msg.Content().Text
				readBytes := messageReadBytes[msg.ID]

				if len(content) < readBytes {
					continue
				}

				part := content[readBytes:]
				if readBytes == 0 {
					part = strings.TrimLeft(part, " \t")
				}

				if strings.TrimSpace(part) != "" {
					fmt.Print(part)
				}
				messageReadBytes[msg.ID] = len(content)
			} else if msg.Role == message.Tool {
				toolCallCount++
				if aiDebug && debugger != nil {
					for _, tr := range msg.ToolResults() {
						debugger.SubHeader(fmt.Sprintf("TOOL [%d]: %s", toolCallCount, tr.Name))
						if tr.IsError {
							debugger.KV("Error", truncateString(tr.Content, 100))
						}
						fmt.Fprint(os.Stderr, debugger.String())
						debugger.Reset()
					}
				}
			}
		}
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
