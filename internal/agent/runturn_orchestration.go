package agent

import (
	stdctx "context"
	"fmt"
)

func (l *AgentLoop) runSingleIteration(ctx stdctx.Context, turnExec *turnExecution, iteration int) (*iterationOutcome, error) {
	l.logger.Info("starting iteration",
		"conversation_id", turnExec.req.ConversationID,
		"turn", turnExec.req.TurnNumber,
		"iteration", iteration,
	)

	if isCancelled(ctx) {
		return nil, l.handleIterationSetupCancellation(turnExec.req.ConversationID, turnExec.req.TurnNumber, iteration, turnExec.completedIterations, ctx.Err())
	}

	iterExec, err := l.prepareIteration(ctx, turnExec, iteration)
	if err != nil {
		return nil, l.normalizeIterationSetupError(ctx, turnExec, iteration, err)
	}

	result, err := l.runProviderIteration(ctx, turnExec, iterExec)
	if err != nil {
		return nil, err
	}

	assistantContentJSON, err := serializeAssistantResponse(result, iteration)
	if err != nil {
		return nil, err
	}

	if !result.HasToolUse() {
		finalResult, err := l.completeTextOnlyIteration(ctx, turnExec, iteration, result, assistantContentJSON)
		if err != nil {
			return nil, err
		}
		return &iterationOutcome{done: true, result: finalResult}, nil
	}

	l.emit(StatusEvent{State: StateExecutingTools, Time: l.now()})

	inflight := newInflightToolTurn(turnExec.req, iteration, turnExec.completedIterations, result, assistantContentJSON)
	validated := l.validateToolCalls(ctx, turnExec, iteration, result, &inflight, iterExec.promptReq.Tools)
	if validated.toolsCancelled {
		return nil, l.handleTurnCancellation(inflight, ctx.Err())
	}

	toolResults, earlyResult, err := l.executeValidToolCalls(ctx, turnExec, iteration, result, &inflight, validated)
	if err != nil {
		return nil, err
	}
	if earlyResult != nil {
		return &iterationOutcome{done: true, result: earlyResult}, nil
	}

	if iterExec.completeAfterTools {
		finalResult, err := l.completeFinalToolIteration(ctx, turnExec, iteration, result, assistantContentJSON, result.ToolCalls, toolResults)
		if err != nil {
			return nil, err
		}
		return &iterationOutcome{done: true, result: finalResult}, nil
	}

	if err := l.completeToolIteration(ctx, turnExec, iteration, assistantContentJSON, result.ToolCalls, toolResults); err != nil {
		return nil, err
	}
	return &iterationOutcome{}, nil
}

func (l *AgentLoop) runTurnIterations(ctx stdctx.Context, turnExec *turnExecution) (*TurnResult, error) {
	for iteration := 1; l.cfg.MaxIterations == 0 || iteration <= l.cfg.MaxIterations; iteration++ {
		outcome, err := l.runSingleIteration(ctx, turnExec, iteration)
		if err != nil {
			return nil, err
		}
		if outcome.done {
			return outcome.result, nil
		}
	}
	return nil, fmt.Errorf("%w (%d)", ErrMaxIterationsExceeded, l.cfg.MaxIterations)
}
