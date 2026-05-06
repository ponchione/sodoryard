package agent

import (
	stdctx "context"
	"fmt"

	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
)

type iterationExecution struct {
	number             int
	history            []db.Message
	toolDefinitions    []provider.ToolDefinition
	disableTools       bool
	completeAfterTools bool
	promptReq          *provider.Request
}

func (l *AgentLoop) historyForIteration(ctx stdctx.Context, turnExec *turnExecution) ([]db.Message, error) {
	if turnExec.historyNeedsRefresh {
		return l.refreshTurnHistory(ctx, turnExec)
	}
	return turnExec.persistedHistory, nil
}

func (l *AgentLoop) refreshTurnHistory(ctx stdctx.Context, turnExec *turnExecution) ([]db.Message, error) {
	history, err := l.conversationManager.ReconstructHistory(ctx, turnExec.req.ConversationID)
	if err != nil {
		return nil, err
	}
	turnExec.persistedHistory = append([]db.Message(nil), history...)
	turnExec.historyNeedsRefresh = false
	return turnExec.persistedHistory, nil
}

func (l *AgentLoop) prepareIteration(ctx stdctx.Context, turnExec *turnExecution, iteration int) (*iterationExecution, error) {
	history, err := l.historyForIteration(ctx, turnExec)
	if err != nil {
		return nil, fmt.Errorf("agent loop: reconstruct history for iteration %d: %w", iteration, err)
	}

	iterExec := &iterationExecution{
		number:          iteration,
		history:         history,
		toolDefinitions: l.toolDefinitions,
	}
	if l.cfg.MaxIterations > 0 && iteration >= l.cfg.MaxIterations {
		iterExec.toolDefinitions = completionToolDefinitions(l.toolDefinitions)
		iterExec.disableTools = len(iterExec.toolDefinitions) == 0
		iterExec.completeAfterTools = len(iterExec.toolDefinitions) > 0
		turnExec.currentTurnMessages = append(turnExec.currentTurnMessages, provider.NewUserMessage(loopDirectiveMessage))
		l.logger.Warn("final iteration reached, limiting tools",
			"conversation_id", turnExec.req.ConversationID,
			"turn", turnExec.req.TurnNumber,
			"iteration", iteration,
			"max_iterations", l.cfg.MaxIterations,
			"completion_tools", toolDefinitionNames(iterExec.toolDefinitions),
		)
	}

	promptReq, err := l.buildIterationRequest(ctx, turnExec, iterExec)
	if err != nil {
		return nil, err
	}
	iterExec.promptReq = promptReq
	return iterExec, nil
}

func (l *AgentLoop) buildIterationRequest(ctx stdctx.Context, turnExec *turnExecution, iterExec *iterationExecution) (*provider.Request, error) {
	buildPrompt := func() (*provider.Request, error) {
		return l.promptBuilder.BuildPrompt(l.buildPromptConfig(
			turnExec.turnCtx.ContextPackage,
			iterExec.history,
			turnExec.currentTurnMessages,
			iterExec.toolDefinitions,
			turnExec.effectiveProvider,
			turnExec.effectiveModel,
			turnExec.req.ModelContextLimit,
			iterExec.disableTools,
			turnExec.req.ConversationID,
			turnExec.req.TurnNumber,
			iterExec.number,
		))
	}

	promptReq, err := buildPrompt()
	if err != nil {
		return nil, fmt.Errorf("agent loop: build prompt for iteration %d: %w", iterExec.number, err)
	}

	if !l.tryPreflightCompression(ctx, turnExec.req.ConversationID, promptReq, turnExec.req.ModelContextLimit) {
		return promptReq, nil
	}

	history, err := l.refreshTurnHistory(ctx, turnExec)
	if err != nil {
		return nil, fmt.Errorf("agent loop: reconstruct history after compression in iteration %d: %w", iterExec.number, err)
	}
	iterExec.history = history

	promptReq, err = buildPrompt()
	if err != nil {
		return nil, fmt.Errorf("agent loop: rebuild prompt after compression in iteration %d: %w", iterExec.number, err)
	}
	return promptReq, nil
}

func (l *AgentLoop) runProviderIteration(ctx stdctx.Context, turnExec *turnExecution, iterExec *iterationExecution) (*streamResult, error) {
	l.emit(StatusEvent{State: StateWaitingForLLM, Time: l.now()})

	result, err := l.streamWithRetry(ctx, iterExec.promptReq, iterExec.number, turnExec.req.ConversationID)
	if err != nil {
		if isCancelled(ctx) {
			if result != nil {
				return nil, l.handleTurnCancellation(partialAssistantCleanupTurn(turnExec, iterExec.number, result), ctx.Err())
			}
			return nil, l.handleIterationSetupCancellation(turnExec.req.ConversationID, turnExec.req.TurnNumber, iterExec.number, turnExec.completedIterations, ctx.Err())
		}

		result, err = l.normalizeOverflowRecovery(ctx, turnExec, iterExec, result, err)
		if err != nil {
			if result != nil && (result.TextContent != "" || len(result.ContentBlocks) > 0) {
				return nil, l.handleTurnStreamFailure(partialAssistantCleanupTurn(turnExec, iterExec.number, result), err)
			}
			return nil, err
		}
	}

	turnExec.totalUsage = turnExec.totalUsage.Add(result.Usage)
	if l.tryPostResponseCompression(ctx, turnExec.req.ConversationID, result.Usage.InputTokens, turnExec.req.ModelContextLimit) {
		turnExec.historyNeedsRefresh = true
	}
	return result, nil
}

func (l *AgentLoop) normalizeOverflowRecovery(
	ctx stdctx.Context,
	turnExec *turnExecution,
	iterExec *iterationExecution,
	result *streamResult,
	err error,
) (*streamResult, error) {
	if err == nil || !l.isContextOverflowError(err) {
		return result, err
	}

	retryResult, retryErr := l.tryEmergencyCompression(
		ctx,
		turnExec,
		iterExec.number,
		iterExec.toolDefinitions,
		iterExec.disableTools,
	)
	if retryResult == nil && retryErr == nil {
		return result, err
	}
	if retryErr != nil {
		return nil, retryErr
	}
	return retryResult, nil
}

func (l *AgentLoop) normalizeIterationSetupError(ctx stdctx.Context, turnExec *turnExecution, iteration int, err error) error {
	if err == nil {
		return nil
	}
	if !isCancelled(ctx) {
		return err
	}
	return l.handleIterationSetupCancellation(
		turnExec.req.ConversationID,
		turnExec.req.TurnNumber,
		iteration,
		turnExec.completedIterations,
		ctx.Err(),
	)
}

func partialAssistantCleanupTurn(turnExec *turnExecution, iteration int, result *streamResult) inflightTurn {
	cleanupTurn := cleanupInflightTurnBase(turnExec, iteration)
	cleanupTurn.AssistantResponseStarted = result != nil && (result.TextContent != "" || len(result.ContentBlocks) > 0)
	cleanupTurn.AssistantMessageContent = assistantContentJSONForCleanup(result)
	return cleanupTurn
}

func assistantContentJSONForCleanup(result *streamResult) string {
	if result == nil || len(result.ContentBlocks) == 0 {
		return ""
	}
	assistantContentJSON, _ := contentBlocksToJSON(sanitizeContentBlocks(result.ContentBlocks))
	return assistantContentJSON
}

func completionToolDefinitions(defs []provider.ToolDefinition) []provider.ToolDefinition {
	filtered := make([]provider.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if isCompletionTool(def.Name) {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func isCompletionTool(name string) bool {
	switch name {
	case "brain_write", "brain_update":
		return true
	default:
		return false
	}
}

func toolDefinitionNames(defs []provider.ToolDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}
