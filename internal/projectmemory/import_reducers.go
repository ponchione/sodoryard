package projectmemory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ponchione/shunter/schema"
	"github.com/ponchione/shunter/types"
)

type ImportSQLiteStateArgs struct {
	Conversations  []Conversation  `json:"conversations"`
	Messages       []Message       `json:"messages"`
	SubCalls       []SubCall       `json:"sub_calls"`
	ToolCalls      []ToolExecution `json:"tool_executions"`
	ContextReports []ContextReport `json:"context_reports"`
	Chains         []Chain         `json:"chains"`
	Steps          []ChainStep     `json:"steps"`
	Events         []ChainEvent    `json:"events"`
	Launches       []Launch        `json:"launches"`
	LaunchPresets  []LaunchPreset  `json:"launch_presets"`
}

type ImportSQLiteStateResult struct {
	Conversations  int `json:"conversations"`
	Messages       int `json:"messages"`
	SubCalls       int `json:"sub_calls"`
	ToolExecutions int `json:"tool_executions"`
	ContextReports int `json:"context_reports"`
	Chains         int `json:"chains"`
	Steps          int `json:"steps"`
	Events         int `json:"events"`
	Launches       int `json:"launches"`
	LaunchPresets  int `json:"launch_presets"`
	Skipped        int `json:"skipped"`
}

func importSQLiteStateReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args ImportSQLiteStateArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	result := ImportSQLiteStateResult{}
	for _, conversation := range args.Conversations {
		inserted, err := importSQLiteConversation(ctx.DB, conversation)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.Conversations++
		} else {
			result.Skipped++
		}
	}
	for _, message := range args.Messages {
		inserted, err := importSQLiteMessage(ctx.DB, message)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.Messages++
		} else {
			result.Skipped++
		}
	}
	for _, subCall := range args.SubCalls {
		inserted, err := importSQLiteSubCall(ctx.DB, subCall)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.SubCalls++
		} else {
			result.Skipped++
		}
	}
	for _, execution := range args.ToolCalls {
		inserted, err := importSQLiteToolExecution(ctx.DB, execution)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.ToolExecutions++
		} else {
			result.Skipped++
		}
	}
	for _, report := range args.ContextReports {
		inserted, err := importSQLiteContextReport(ctx.DB, report)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.ContextReports++
		} else {
			result.Skipped++
		}
	}
	for _, chain := range args.Chains {
		inserted, err := importSQLiteChain(ctx.DB, chain)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.Chains++
		} else {
			result.Skipped++
		}
	}
	for _, step := range args.Steps {
		inserted, err := importSQLiteStep(ctx.DB, step)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.Steps++
		} else {
			result.Skipped++
		}
	}
	for _, event := range args.Events {
		inserted, err := importSQLiteEvent(ctx.DB, event)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.Events++
		} else {
			result.Skipped++
		}
	}
	for _, launch := range args.Launches {
		inserted, err := importSQLiteLaunch(ctx.DB, launch)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.Launches++
		} else {
			result.Skipped++
		}
	}
	for _, preset := range args.LaunchPresets {
		inserted, err := importSQLiteLaunchPreset(ctx.DB, preset)
		if err != nil {
			return nil, err
		}
		if inserted {
			result.LaunchPresets++
		} else {
			result.Skipped++
		}
	}
	return json.Marshal(result)
}

func importSQLiteConversation(db types.ReducerDB, conversation Conversation) (bool, error) {
	conversation.ID = strings.TrimSpace(conversation.ID)
	conversation.ProjectID = strings.TrimSpace(conversation.ProjectID)
	if conversation.ID == "" {
		return false, fmt.Errorf("sqlite conversation id is required")
	}
	if conversation.ProjectID == "" {
		return false, fmt.Errorf("sqlite conversation project id is required")
	}
	if _, _, found := findConversationByID(db, conversation.ID); found {
		return false, nil
	}
	conversation.SettingsJSON = defaultString(conversation.SettingsJSON, emptyJSONObject)
	if _, err := db.Insert(uint32(tableConversations), conversationRow(conversation)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteMessage(db types.ReducerDB, message Message) (bool, error) {
	message.ID = strings.TrimSpace(message.ID)
	message.ConversationID = strings.TrimSpace(message.ConversationID)
	if message.ID == "" {
		message.ID = MessageID(message.ConversationID, message.Sequence)
	}
	if _, conversation, found := findConversationByID(db, message.ConversationID); !found || conversation.Deleted {
		return false, fmt.Errorf("sqlite message conversation not found: %s", message.ConversationID)
	}
	if _, _, found := firstRow(db.SeekIndex(uint32(tableMessages), uint32(indexMessagesPrimary), types.NewString(message.ID))); found {
		return false, nil
	}
	message.SummaryOfJSON = defaultString(message.SummaryOfJSON, emptyJSONArray)
	message.MetadataJSON = defaultString(message.MetadataJSON, emptyJSONObject)
	if _, err := db.Insert(uint32(tableMessages), messageRow(message)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteSubCall(db types.ReducerDB, subCall SubCall) (bool, error) {
	subCall.ID = strings.TrimSpace(subCall.ID)
	if subCall.ID == "" {
		subCall.ID = SubCallID(subCall.ConversationID, subCall.MessageID, fmt.Sprint(subCall.TurnNumber), fmt.Sprint(subCall.Iteration), subCall.Provider, subCall.Model, subCall.Purpose, fmt.Sprint(subCall.CompletedAtUS), fmt.Sprint(subCall.TokensIn), fmt.Sprint(subCall.TokensOut), subCall.Error)
	}
	if strings.TrimSpace(subCall.Provider) == "" {
		return false, fmt.Errorf("sqlite sub-call provider is required")
	}
	if strings.TrimSpace(subCall.Model) == "" {
		return false, fmt.Errorf("sqlite sub-call model is required")
	}
	if strings.TrimSpace(subCall.Purpose) == "" {
		return false, fmt.Errorf("sqlite sub-call purpose is required")
	}
	if _, _, found := firstRow(db.SeekIndex(uint32(tableSubCalls), uint32(indexSubCallsPrimary), types.NewString(subCall.ID))); found {
		return false, nil
	}
	subCall.MetadataJSON = defaultString(subCall.MetadataJSON, emptyJSONObject)
	if _, err := db.Insert(uint32(tableSubCalls), subCallRow(subCall)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteToolExecution(db types.ReducerDB, execution ToolExecution) (bool, error) {
	execution.ID = strings.TrimSpace(execution.ID)
	execution.ConversationID = strings.TrimSpace(execution.ConversationID)
	if execution.ConversationID == "" {
		return false, fmt.Errorf("sqlite tool execution conversation id is required")
	}
	if _, conversation, found := findConversationByID(db, execution.ConversationID); !found || conversation.Deleted {
		return false, fmt.Errorf("sqlite tool execution conversation not found: %s", execution.ConversationID)
	}
	if execution.ID == "" {
		execution.ID = ToolExecutionID(execution.ConversationID, execution.TurnNumber, execution.Iteration, execution.ToolUseID, execution.ToolName)
	}
	if _, _, found := firstRow(db.SeekIndex(uint32(tableToolExecutions), uint32(indexToolExecutionsPrimary), types.NewString(execution.ID))); found {
		return false, nil
	}
	execution.MetadataJSON = defaultString(execution.MetadataJSON, emptyJSONObject)
	if _, err := db.Insert(uint32(tableToolExecutions), toolExecutionRow(execution)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteContextReport(db types.ReducerDB, report ContextReport) (bool, error) {
	report.ID = strings.TrimSpace(report.ID)
	report.ConversationID = strings.TrimSpace(report.ConversationID)
	if report.ConversationID == "" {
		return false, fmt.Errorf("sqlite context report conversation id is required")
	}
	if _, conversation, found := findConversationByID(db, report.ConversationID); !found || conversation.Deleted {
		return false, fmt.Errorf("sqlite context report conversation not found: %s", report.ConversationID)
	}
	if report.TurnNumber == 0 {
		return false, fmt.Errorf("sqlite context report turn number is required")
	}
	expectedID := ContextReportID(report.ConversationID, report.TurnNumber)
	if report.ID == "" {
		report.ID = expectedID
	}
	if report.ID != expectedID {
		return false, fmt.Errorf("sqlite context report id must match conversation and turn")
	}
	if _, _, found := firstRow(db.SeekIndex(uint32(tableContextReports), uint32(indexContextReportsPrimary), types.NewString(report.ID))); found {
		return false, nil
	}
	report.RequestJSON = defaultString(report.RequestJSON, emptyJSONObject)
	report.ReportJSON = defaultString(report.ReportJSON, emptyJSONObject)
	report.QualityJSON = defaultString(report.QualityJSON, emptyJSONObject)
	if _, err := db.Insert(uint32(tableContextReports), contextReportRow(report)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteChain(db types.ReducerDB, chain Chain) (bool, error) {
	chain.ID = strings.TrimSpace(chain.ID)
	if chain.ID == "" {
		return false, fmt.Errorf("sqlite chain id is required")
	}
	if _, _, found := findChainByID(db, chain.ID); found {
		return false, nil
	}
	if _, err := db.Insert(uint32(tableChains), chainRow(chain)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteStep(db types.ReducerDB, step ChainStep) (bool, error) {
	step.ID = strings.TrimSpace(step.ID)
	step.ChainID = strings.TrimSpace(step.ChainID)
	if step.ID == "" {
		return false, fmt.Errorf("sqlite step id is required")
	}
	if _, _, found := findChainByID(db, step.ChainID); !found {
		return false, fmt.Errorf("sqlite step chain not found: %s", step.ChainID)
	}
	if _, _, found := findStepByID(db, step.ID); found {
		return false, nil
	}
	if _, err := db.Insert(uint32(tableSteps), chainStepRow(step)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteEvent(db types.ReducerDB, event ChainEvent) (bool, error) {
	event.ID = strings.TrimSpace(event.ID)
	event.ChainID = strings.TrimSpace(event.ChainID)
	if event.ChainID == "" {
		return false, fmt.Errorf("sqlite event chain id is required")
	}
	if _, _, found := findChainByID(db, event.ChainID); !found {
		return false, fmt.Errorf("sqlite event chain not found: %s", event.ChainID)
	}
	if event.ID == "" {
		event.ID = ChainEventID(event.ChainID, event.Sequence)
	}
	if _, _, found := firstRow(db.SeekIndex(uint32(tableEvents), uint32(indexEventsPrimary), types.NewString(event.ID))); found {
		return false, nil
	}
	if _, err := db.Insert(uint32(tableEvents), chainEventRow(event)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteLaunch(db types.ReducerDB, launch Launch) (bool, error) {
	launch.ProjectID = strings.TrimSpace(launch.ProjectID)
	launch.LaunchID = strings.TrimSpace(launch.LaunchID)
	if launch.ProjectID == "" {
		return false, fmt.Errorf("sqlite launch project id is required")
	}
	if launch.LaunchID == "" {
		return false, fmt.Errorf("sqlite launch id is required")
	}
	if launch.ID == "" {
		launch.ID = ProjectLaunchID(launch.ProjectID, launch.LaunchID)
	}
	if _, _, found := firstRow(db.SeekIndex(uint32(tableLaunches), uint32(indexLaunchesPrimary), types.NewString(launch.ID))); found {
		return false, nil
	}
	if _, err := db.Insert(uint32(tableLaunches), launchRow(launch)); err != nil {
		return false, err
	}
	return true, nil
}

func importSQLiteLaunchPreset(db types.ReducerDB, preset LaunchPreset) (bool, error) {
	preset.ProjectID = strings.TrimSpace(preset.ProjectID)
	preset.Name = strings.TrimSpace(preset.Name)
	if preset.ProjectID == "" {
		return false, fmt.Errorf("sqlite launch preset project id is required")
	}
	if preset.Name == "" {
		return false, fmt.Errorf("sqlite launch preset name is required")
	}
	if preset.ID == "" {
		preset.ID = ProjectLaunchPresetID(preset.ProjectID, preset.Name)
	}
	if preset.PresetID == "" {
		preset.PresetID = "custom:" + preset.Name
	}
	if _, _, found := firstRow(db.SeekIndex(uint32(tableLaunchPresets), uint32(indexLaunchPresetsPrimary), types.NewString(preset.ID))); found {
		return false, nil
	}
	if _, err := db.Insert(uint32(tableLaunchPresets), launchPresetRow(preset)); err != nil {
		return false, err
	}
	return true, nil
}
