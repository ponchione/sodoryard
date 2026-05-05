package main

import (
	stdctx "context"
	"fmt"
	"os"
	"strings"

	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func verifySQLiteProjectMemory(ctx stdctx.Context, sqlitePath string, target *projectmemory.BrainBackend) (projectmemory.ImportSQLiteStateResult, error) {
	if strings.TrimSpace(sqlitePath) == "" {
		return projectmemory.ImportSQLiteStateResult{}, fmt.Errorf("source SQLite path is required")
	}
	if target == nil {
		return projectmemory.ImportSQLiteStateResult{}, fmt.Errorf("Shunter project memory target is required")
	}
	if _, err := os.Stat(sqlitePath); err != nil {
		return projectmemory.ImportSQLiteStateResult{}, fmt.Errorf("stat source SQLite database: %w", err)
	}
	database, err := openSQLiteMigrationDB(ctx, sqlitePath)
	if err != nil {
		return projectmemory.ImportSQLiteStateResult{}, fmt.Errorf("open source SQLite database: %w", err)
	}
	defer database.Close()

	state, err := readSQLiteProjectMemoryState(ctx, database)
	if err != nil {
		return projectmemory.ImportSQLiteStateResult{}, err
	}
	return verifySQLiteProjectMemoryState(ctx, state, target)
}

func verifySQLiteProjectMemoryState(ctx stdctx.Context, state projectmemory.ImportSQLiteStateArgs, target *projectmemory.BrainBackend) (projectmemory.ImportSQLiteStateResult, error) {
	result := projectmemory.ImportSQLiteStateResult{}
	for _, expected := range state.Conversations {
		actual, found, err := target.ReadConversation(ctx, expected.ID)
		if err != nil {
			return result, fmt.Errorf("read Shunter conversation %s: %w", expected.ID, err)
		}
		if !found {
			return result, fmt.Errorf("sqlite conversation missing in Shunter: %s", expected.ID)
		}
		if actual != expected {
			return result, sqliteVerifyMismatch("conversation", expected.ID, actual, expected)
		}
		result.Conversations++
	}

	messagesByConversation := map[string][]projectmemory.Message{}
	for _, expected := range state.Messages {
		messagesByConversation[expected.ConversationID] = append(messagesByConversation[expected.ConversationID], expected)
	}
	for conversationID, expectedMessages := range messagesByConversation {
		actualMessages, err := target.ListMessages(ctx, conversationID, true)
		if err != nil {
			return result, fmt.Errorf("list Shunter messages for %s: %w", conversationID, err)
		}
		actualByID := mapMessagesByID(actualMessages)
		for _, expected := range expectedMessages {
			actual, found := actualByID[expected.ID]
			if !found {
				return result, fmt.Errorf("sqlite message missing in Shunter: %s", expected.ID)
			}
			if actual != expected {
				return result, sqliteVerifyMismatch("message", expected.ID, actual, expected)
			}
			result.Messages++
		}
	}

	actualSubCalls, err := target.ListSubCalls(ctx, "")
	if err != nil {
		return result, fmt.Errorf("list Shunter provider subcalls: %w", err)
	}
	actualSubCallsByID := mapSubCallsByID(actualSubCalls)
	for _, expected := range state.SubCalls {
		actual, found := actualSubCallsByID[expected.ID]
		if !found {
			return result, fmt.Errorf("sqlite provider subcall missing in Shunter: %s", expected.ID)
		}
		if actual != expected {
			return result, sqliteVerifyMismatch("provider subcall", expected.ID, actual, expected)
		}
		result.SubCalls++
	}

	actualToolCalls, err := target.ListToolExecutions(ctx, "")
	if err != nil {
		return result, fmt.Errorf("list Shunter tool executions: %w", err)
	}
	actualToolCallsByID := mapToolExecutionsByID(actualToolCalls)
	for _, expected := range state.ToolCalls {
		actual, found := actualToolCallsByID[expected.ID]
		if !found {
			return result, fmt.Errorf("sqlite tool execution missing in Shunter: %s", expected.ID)
		}
		if actual != expected {
			return result, sqliteVerifyMismatch("tool execution", expected.ID, actual, expected)
		}
		result.ToolExecutions++
	}

	for _, expected := range state.ContextReports {
		actual, found, err := target.ReadContextReport(ctx, expected.ConversationID, expected.TurnNumber)
		if err != nil {
			return result, fmt.Errorf("read Shunter context report %s: %w", expected.ID, err)
		}
		if !found {
			return result, fmt.Errorf("sqlite context report missing in Shunter: %s", expected.ID)
		}
		if actual != expected {
			return result, sqliteVerifyMismatch("context report", expected.ID, actual, expected)
		}
		result.ContextReports++
	}

	for _, expected := range state.Chains {
		actual, found, err := target.ReadChain(ctx, expected.ID)
		if err != nil {
			return result, fmt.Errorf("read Shunter chain %s: %w", expected.ID, err)
		}
		if !found {
			return result, fmt.Errorf("sqlite chain missing in Shunter: %s", expected.ID)
		}
		if actual != expected {
			return result, sqliteVerifyMismatch("chain", expected.ID, actual, expected)
		}
		result.Chains++
	}

	stepsByChain := map[string][]projectmemory.ChainStep{}
	for _, expected := range state.Steps {
		stepsByChain[expected.ChainID] = append(stepsByChain[expected.ChainID], expected)
	}
	for chainID, expectedSteps := range stepsByChain {
		actualSteps, err := target.ListChainSteps(ctx, chainID)
		if err != nil {
			return result, fmt.Errorf("list Shunter chain steps for %s: %w", chainID, err)
		}
		actualByID := mapChainStepsByID(actualSteps)
		for _, expected := range expectedSteps {
			actual, found := actualByID[expected.ID]
			if !found {
				return result, fmt.Errorf("sqlite chain step missing in Shunter: %s", expected.ID)
			}
			if actual != expected {
				return result, sqliteVerifyMismatch("chain step", expected.ID, actual, expected)
			}
			result.Steps++
		}
	}

	eventsByChain := map[string][]projectmemory.ChainEvent{}
	for _, expected := range state.Events {
		eventsByChain[expected.ChainID] = append(eventsByChain[expected.ChainID], expected)
	}
	for chainID, expectedEvents := range eventsByChain {
		actualEvents, err := target.ListChainEvents(ctx, chainID)
		if err != nil {
			return result, fmt.Errorf("list Shunter chain events for %s: %w", chainID, err)
		}
		actualByID := mapChainEventsByID(actualEvents)
		for _, expected := range expectedEvents {
			actual, found := actualByID[expected.ID]
			if !found {
				return result, fmt.Errorf("sqlite chain event missing in Shunter: %s", expected.ID)
			}
			if actual != expected {
				return result, sqliteVerifyMismatch("chain event", expected.ID, actual, expected)
			}
			result.Events++
		}
	}

	for _, expected := range state.Launches {
		actual, found, err := target.ReadLaunch(ctx, expected.ProjectID, expected.LaunchID)
		if err != nil {
			return result, fmt.Errorf("read Shunter launch %s: %w", expected.ID, err)
		}
		if !found {
			return result, fmt.Errorf("sqlite launch draft missing in Shunter: %s", expected.ID)
		}
		if actual != expected {
			return result, sqliteVerifyMismatch("launch draft", expected.ID, actual, expected)
		}
		result.Launches++
	}

	presetsByProject := map[string][]projectmemory.LaunchPreset{}
	for _, expected := range state.LaunchPresets {
		presetsByProject[expected.ProjectID] = append(presetsByProject[expected.ProjectID], expected)
	}
	for projectID, expectedPresets := range presetsByProject {
		actualPresets, err := target.ListLaunchPresets(ctx, projectID)
		if err != nil {
			return result, fmt.Errorf("list Shunter launch presets for %s: %w", projectID, err)
		}
		actualByID := mapLaunchPresetsByID(actualPresets)
		for _, expected := range expectedPresets {
			actual, found := actualByID[expected.ID]
			if !found {
				return result, fmt.Errorf("sqlite launch preset missing in Shunter: %s", expected.ID)
			}
			if actual != expected {
				return result, sqliteVerifyMismatch("launch preset", expected.ID, actual, expected)
			}
			result.LaunchPresets++
		}
	}
	return result, nil
}

func sqliteVerifyMismatch(kind string, id string, actual any, expected any) error {
	return fmt.Errorf("sqlite %s mismatch %s: Shunter=%+v source=%+v", kind, id, actual, expected)
}

func mapMessagesByID(messages []projectmemory.Message) map[string]projectmemory.Message {
	byID := make(map[string]projectmemory.Message, len(messages))
	for _, message := range messages {
		byID[message.ID] = message
	}
	return byID
}

func mapSubCallsByID(subCalls []projectmemory.SubCall) map[string]projectmemory.SubCall {
	byID := make(map[string]projectmemory.SubCall, len(subCalls))
	for _, subCall := range subCalls {
		byID[subCall.ID] = subCall
	}
	return byID
}

func mapToolExecutionsByID(executions []projectmemory.ToolExecution) map[string]projectmemory.ToolExecution {
	byID := make(map[string]projectmemory.ToolExecution, len(executions))
	for _, execution := range executions {
		byID[execution.ID] = execution
	}
	return byID
}

func mapChainStepsByID(steps []projectmemory.ChainStep) map[string]projectmemory.ChainStep {
	byID := make(map[string]projectmemory.ChainStep, len(steps))
	for _, step := range steps {
		byID[step.ID] = step
	}
	return byID
}

func mapChainEventsByID(events []projectmemory.ChainEvent) map[string]projectmemory.ChainEvent {
	byID := make(map[string]projectmemory.ChainEvent, len(events))
	for _, event := range events {
		byID[event.ID] = event
	}
	return byID
}

func mapLaunchPresetsByID(presets []projectmemory.LaunchPreset) map[string]projectmemory.LaunchPreset {
	byID := make(map[string]projectmemory.LaunchPreset, len(presets))
	for _, preset := range presets {
		byID[preset.ID] = preset
	}
	return byID
}
