package projectmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/shunter"
	"github.com/ponchione/shunter/types"
)

func (r *Runtime) ReadDocument(ctx context.Context, path string) (Document, string, error) {
	normalized, err := normalizeDocumentPath(path)
	if err != nil {
		return Document{}, "", err
	}
	var doc Document
	var content string
	var found bool
	err = r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableDocuments, indexDocumentsPrimary, types.NewString(normalized)) {
			doc = decodeDocumentRow(row)
			found = true
			break
		}
		if !found || doc.Deleted {
			return nil
		}
		var chunks []documentChunk
		for _, row := range view.SeekIndex(tableDocumentChunks, indexDocumentChunksPath, types.NewString(normalized)) {
			chunks = append(chunks, decodeChunkRow(row))
		}
		content = joinDocumentChunks(chunks)
		return nil
	})
	if err != nil {
		return Document{}, "", err
	}
	if !found || doc.Deleted {
		return Document{}, "", fmt.Errorf("Document not found: %s", normalized)
	}
	return doc, content, nil
}

func (r *Runtime) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	prefix := strings.Trim(filepathSlash(directory), "/")
	if prefix != "" {
		prefix += "/"
	}
	var paths []string
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.TableScan(tableDocuments) {
			doc := decodeDocumentRow(row)
			if doc.Deleted {
				continue
			}
			if prefix != "" && !strings.HasPrefix(doc.Path, prefix) {
				continue
			}
			paths = append(paths, doc.Path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *Runtime) ListDocumentLinks(ctx context.Context, sourcePath string, targetPath string) ([]DocumentLink, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	targetPath = strings.TrimSpace(targetPath)
	if sourcePath != "" {
		normalized, err := normalizeDocumentPath(sourcePath)
		if err != nil {
			return nil, err
		}
		sourcePath = normalized
	}
	if targetPath != "" {
		targetPath = normalizeLinkTarget(targetPath)
	}
	var links []DocumentLink
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		switch {
		case sourcePath != "":
			for _, row := range view.SeekIndex(tableDocumentLinks, indexDocumentLinksSource, types.NewString(sourcePath)) {
				link := decodeDocumentLinkRow(row)
				if targetPath != "" && link.TargetPath != targetPath {
					continue
				}
				links = append(links, link)
			}
		case targetPath != "":
			for _, row := range view.SeekIndex(tableDocumentLinks, indexDocumentLinksTarget, types.NewString(targetPath)) {
				links = append(links, decodeDocumentLinkRow(row))
			}
		default:
			for _, row := range view.TableScan(tableDocumentLinks) {
				links = append(links, decodeDocumentLinkRow(row))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].SourcePath == links[j].SourcePath {
			if links[i].TargetPath == links[j].TargetPath {
				return links[i].LinkText < links[j].LinkText
			}
			return links[i].TargetPath < links[j].TargetPath
		}
		return links[i].SourcePath < links[j].SourcePath
	})
	return links, nil
}

func (r *Runtime) SearchDocuments(ctx context.Context, query string, maxResults int) ([]SearchHit, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	normalizedQuery := normalizeKeyword(query)
	if normalizedQuery == "" {
		return nil, nil
	}
	var hits []SearchHit
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.TableScan(tableDocuments) {
			doc := decodeDocumentRow(row)
			if doc.Deleted {
				continue
			}
			var chunks []documentChunk
			for _, chunkRow := range view.SeekIndex(tableDocumentChunks, indexDocumentChunksPath, types.NewString(doc.Path)) {
				chunks = append(chunks, decodeChunkRow(chunkRow))
			}
			content := joinDocumentChunks(chunks)
			score := keywordScore(normalizedQuery, doc, content)
			if score == 0 {
				continue
			}
			hits = append(hits, SearchHit{
				Path:    doc.Path,
				Snippet: snippetAround(content, normalizedQuery),
				Score:   score,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return hits, nil
}

func (r *Runtime) ReadBrainIndexState(ctx context.Context) (BrainIndexState, bool, error) {
	var state BrainIndexState
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableBrainIndexState, indexBrainIndexStatePrimary, types.NewString(DefaultProjectID)) {
			state = decodeBrainIndexStateRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return BrainIndexState{}, false, err
	}
	return state, found, nil
}

func (r *Runtime) ListBrainIndexChunks(ctx context.Context, documentPath string) ([]BrainIndexChunk, error) {
	documentPath = strings.TrimSpace(documentPath)
	if documentPath != "" {
		normalized, err := normalizeDocumentPath(documentPath)
		if err != nil {
			return nil, err
		}
		documentPath = normalized
	}
	var chunks []BrainIndexChunk
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		if documentPath != "" {
			for _, row := range view.SeekIndex(tableBrainIndexChunks, indexBrainIndexChunksDocument, types.NewString(documentPath)) {
				chunks = append(chunks, decodeBrainIndexChunkRow(row))
			}
			return nil
		}
		for _, row := range view.TableScan(tableBrainIndexChunks) {
			chunks = append(chunks, decodeBrainIndexChunkRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].DocumentPath == chunks[j].DocumentPath {
			return chunks[i].ChunkID < chunks[j].ChunkID
		}
		return chunks[i].DocumentPath < chunks[j].DocumentPath
	})
	return chunks, nil
}

func (r *Runtime) ReadCodeIndexState(ctx context.Context) (CodeIndexState, bool, error) {
	var state CodeIndexState
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableCodeIndexState, indexCodeIndexStatePrimary, types.NewString(DefaultProjectID)) {
			state = decodeCodeIndexStateRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return CodeIndexState{}, false, err
	}
	return state, found, nil
}

func (r *Runtime) ListCodeFileIndexStates(ctx context.Context) ([]CodeFileIndexState, error) {
	var states []CodeFileIndexState
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableCodeFileIndexState, indexCodeFileIndexStateProject, types.NewString(DefaultProjectID)) {
			states = append(states, decodeCodeFileIndexStateRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].FilePath < states[j].FilePath
	})
	return states, nil
}

func (r *Runtime) ReadConversation(ctx context.Context, id string) (Conversation, bool, error) {
	var conversation Conversation
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableConversations, indexConversationsPrimary, types.NewString(strings.TrimSpace(id))) {
			conversation = decodeConversationRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return Conversation{}, false, err
	}
	if !found || conversation.Deleted {
		return Conversation{}, false, nil
	}
	return conversation, true, nil
}

func (r *Runtime) ListConversations(ctx context.Context, projectID string, limit, offset int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var conversations []Conversation
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableConversations, indexConversationsProject, types.NewString(projectID)) {
			conversation := decodeConversationRow(row)
			if conversation.Deleted {
				continue
			}
			conversations = append(conversations, conversation)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(conversations, func(i, j int) bool {
		if conversations[i].UpdatedAtUS == conversations[j].UpdatedAtUS {
			return conversations[i].ID < conversations[j].ID
		}
		return conversations[i].UpdatedAtUS > conversations[j].UpdatedAtUS
	})
	if offset >= len(conversations) {
		return nil, nil
	}
	end := offset + limit
	if end > len(conversations) {
		end = len(conversations)
	}
	return conversations[offset:end], nil
}

func (r *Runtime) CountConversations(ctx context.Context, projectID string) (int64, error) {
	var count int64
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableConversations, indexConversationsProject, types.NewString(projectID)) {
			if !decodeConversationRow(row).Deleted {
				count++
			}
		}
		return nil
	})
	return count, err
}

func (r *Runtime) ListMessages(ctx context.Context, conversationID string, includeCompressed bool) ([]Message, error) {
	var messages []Message
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableMessages, indexMessagesConversation, types.NewString(conversationID)) {
			message := decodeMessageRow(row)
			if !message.Visible {
				continue
			}
			if message.Compressed && !includeCompressed {
				continue
			}
			messages = append(messages, message)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortMessages(messages)
	return messages, nil
}

func (r *Runtime) GetMessagePage(ctx context.Context, conversationID string, limit, offset int) ([]Message, error) {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	messages, err := r.ListMessages(ctx, conversationID, true)
	if err != nil {
		return nil, err
	}
	if offset >= len(messages) {
		return nil, nil
	}
	start := len(messages) - offset - limit
	if start < 0 {
		start = 0
	}
	end := len(messages) - offset
	if end < 0 {
		end = 0
	}
	return messages[start:end], nil
}

func (r *Runtime) NextTurnNumber(ctx context.Context, conversationID string) (int, error) {
	messages, err := r.ListMessages(ctx, conversationID, true)
	if err != nil {
		return 0, err
	}
	maxTurn := uint32(0)
	for _, message := range messages {
		if message.TurnNumber > maxTurn {
			maxTurn = message.TurnNumber
		}
	}
	return int(maxTurn) + 1, nil
}

func (r *Runtime) SearchConversations(ctx context.Context, projectID string, query string, maxResults int) ([]ConversationSearchHit, error) {
	if maxResults <= 0 {
		maxResults = 20
	}
	normalizedQuery := normalizeKeyword(query)
	if normalizedQuery == "" {
		return nil, nil
	}
	conversations, err := r.ListConversations(ctx, projectID, int(^uint(0)>>1), 0)
	if err != nil {
		return nil, err
	}
	type scoredHit struct {
		hit   ConversationSearchHit
		score int
		order int
	}
	best := make(map[string]scoredHit)
	order := 0
	err = r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, conversation := range conversations {
			titleMatch := strings.Contains(normalizeKeyword(conversation.Title), normalizedQuery)
			for _, row := range view.SeekIndex(tableMessages, indexMessagesConversation, types.NewString(conversation.ID)) {
				message := decodeMessageRow(row)
				if !message.Visible {
					continue
				}
				contentMatch := strings.Contains(normalizeKeyword(message.Content), normalizedQuery)
				if !titleMatch && !contentMatch {
					continue
				}
				score := conversationSearchScore(message.Role, titleMatch, contentMatch)
				hit := ConversationSearchHit{
					ID:          conversation.ID,
					Title:       conversation.Title,
					UpdatedAtUS: conversation.UpdatedAtUS,
					Role:        message.Role,
					Snippet:     snippetAround(message.Content, normalizedQuery),
				}
				if hit.Snippet == "" && titleMatch {
					hit.Snippet = conversation.Title
				}
				current, exists := best[conversation.ID]
				if !exists || score > current.score {
					best[conversation.ID] = scoredHit{hit: hit, score: score, order: order}
				}
				order++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	hits := make([]scoredHit, 0, len(best))
	for _, hit := range best {
		hits = append(hits, hit)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score == hits[j].score {
			return hits[i].order < hits[j].order
		}
		return hits[i].score > hits[j].score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	out := make([]ConversationSearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.hit)
	}
	return out, nil
}

func (r *Runtime) ListSubCalls(ctx context.Context, conversationID string) ([]SubCall, error) {
	var subCalls []SubCall
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		if strings.TrimSpace(conversationID) == "" {
			for _, row := range view.TableScan(tableSubCalls) {
				subCalls = append(subCalls, decodeSubCallRow(row))
			}
			return nil
		}
		for _, row := range view.SeekIndex(tableSubCalls, indexSubCallsConversation, types.NewString(conversationID)) {
			subCalls = append(subCalls, decodeSubCallRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortSubCalls(subCalls)
	return subCalls, nil
}

func (r *Runtime) ListTurnSubCalls(ctx context.Context, conversationID string, turnNumber uint32) ([]SubCall, error) {
	subCalls, err := r.ListSubCalls(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]SubCall, 0, len(subCalls))
	for _, subCall := range subCalls {
		if subCall.TurnNumber == turnNumber {
			out = append(out, subCall)
		}
	}
	return out, nil
}

func (r *Runtime) ListToolExecutions(ctx context.Context, conversationID string) ([]ToolExecution, error) {
	var executions []ToolExecution
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		if strings.TrimSpace(conversationID) == "" {
			for _, row := range view.TableScan(tableToolExecutions) {
				executions = append(executions, decodeToolExecutionRow(row))
			}
			return nil
		}
		for _, row := range view.SeekIndex(tableToolExecutions, indexToolExecutionsConversation, types.NewString(conversationID)) {
			executions = append(executions, decodeToolExecutionRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortToolExecutions(executions)
	return executions, nil
}

func (r *Runtime) ListTurnToolExecutions(ctx context.Context, conversationID string, turnNumber uint32) ([]ToolExecution, error) {
	executions, err := r.ListToolExecutions(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]ToolExecution, 0, len(executions))
	for _, execution := range executions {
		if execution.TurnNumber == turnNumber {
			out = append(out, execution)
		}
	}
	return out, nil
}

func (r *Runtime) ReadContextReport(ctx context.Context, conversationID string, turnNumber uint32) (ContextReport, bool, error) {
	if strings.TrimSpace(conversationID) == "" {
		return ContextReport{}, false, fmt.Errorf("context report conversation id is required")
	}
	if turnNumber == 0 {
		return ContextReport{}, false, fmt.Errorf("context report turn number is required")
	}
	var report ContextReport
	var found bool
	id := ContextReportID(conversationID, turnNumber)
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableContextReports, indexContextReportsPrimary, types.NewString(id)) {
			report = decodeContextReportRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return ContextReport{}, false, err
	}
	return report, found, nil
}

func (r *Runtime) ListContextReports(ctx context.Context, conversationID string) ([]ContextReport, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("context report conversation id is required")
	}
	var reports []ContextReport
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableContextReports, indexContextReportsConversation, types.NewString(conversationID)) {
			reports = append(reports, decodeContextReportRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortContextReports(reports)
	return reports, nil
}

func (r *Runtime) ReadChain(ctx context.Context, id string) (Chain, bool, error) {
	if strings.TrimSpace(id) == "" {
		return Chain{}, false, fmt.Errorf("chain id is required")
	}
	var chain Chain
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableChains, indexChainsPrimary, types.NewString(strings.TrimSpace(id))) {
			chain = decodeChainRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return Chain{}, false, err
	}
	return chain, found, nil
}

func (r *Runtime) ListChains(ctx context.Context, limit int) ([]Chain, error) {
	chains := make([]Chain, 0)
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.TableScan(tableChains) {
			chains = append(chains, decodeChainRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortChains(chains)
	if limit > 0 && len(chains) > limit {
		chains = chains[:limit]
	}
	return chains, nil
}

func (r *Runtime) ReadStep(ctx context.Context, id string) (ChainStep, bool, error) {
	if strings.TrimSpace(id) == "" {
		return ChainStep{}, false, fmt.Errorf("step id is required")
	}
	var step ChainStep
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableSteps, indexStepsPrimary, types.NewString(strings.TrimSpace(id))) {
			step = decodeChainStepRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return ChainStep{}, false, err
	}
	return step, found, nil
}

func (r *Runtime) ListChainSteps(ctx context.Context, chainID string) ([]ChainStep, error) {
	steps := make([]ChainStep, 0)
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableSteps, indexStepsChain, types.NewString(strings.TrimSpace(chainID))) {
			steps = append(steps, decodeChainStepRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortChainSteps(steps)
	return steps, nil
}

func (r *Runtime) ListChainEvents(ctx context.Context, chainID string) ([]ChainEvent, error) {
	return r.ListChainEventsSince(ctx, chainID, 0)
}

func (r *Runtime) ListChainEventsSince(ctx context.Context, chainID string, afterSequence uint64) ([]ChainEvent, error) {
	events := make([]ChainEvent, 0)
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableEvents, indexEventsChain, types.NewString(strings.TrimSpace(chainID))) {
			event := decodeChainEventRow(row)
			if event.Sequence <= afterSequence {
				continue
			}
			events = append(events, event)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortChainEvents(events)
	return events, nil
}

func (r *Runtime) ReadLaunch(ctx context.Context, projectID string, launchID string) (Launch, bool, error) {
	projectID = strings.TrimSpace(projectID)
	launchID = strings.TrimSpace(launchID)
	if projectID == "" {
		return Launch{}, false, fmt.Errorf("launch project id is required")
	}
	if launchID == "" {
		return Launch{}, false, fmt.Errorf("launch id is required")
	}
	id := ProjectLaunchID(projectID, launchID)
	var launch Launch
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableLaunches, indexLaunchesPrimary, types.NewString(id)) {
			launch = decodeLaunchRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return Launch{}, false, err
	}
	return launch, found, nil
}

func (r *Runtime) ListLaunchPresets(ctx context.Context, projectID string) ([]LaunchPreset, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("launch preset project id is required")
	}
	presets := make([]LaunchPreset, 0)
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableLaunchPresets, indexLaunchPresetsProject, types.NewString(projectID)) {
			presets = append(presets, decodeLaunchPresetRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortLaunchPresets(presets)
	return presets, nil
}

type SearchHit struct {
	Path    string
	Snippet string
	Score   float64
}

type ConversationSearchHit struct {
	ID          string
	Title       string
	UpdatedAtUS uint64
	Role        string
	Snippet     string
}

func sortMessages(messages []Message) {
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].Sequence == messages[j].Sequence {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].Sequence < messages[j].Sequence
	})
}

func sortSubCalls(subCalls []SubCall) {
	sort.Slice(subCalls, func(i, j int) bool {
		if subCalls[i].CompletedAtUS == subCalls[j].CompletedAtUS {
			return subCalls[i].ID < subCalls[j].ID
		}
		return subCalls[i].CompletedAtUS < subCalls[j].CompletedAtUS
	})
}

func sortToolExecutions(executions []ToolExecution) {
	sort.Slice(executions, func(i, j int) bool {
		if executions[i].CompletedAtUS == executions[j].CompletedAtUS {
			return executions[i].ID < executions[j].ID
		}
		return executions[i].CompletedAtUS < executions[j].CompletedAtUS
	})
}

func sortContextReports(reports []ContextReport) {
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].TurnNumber == reports[j].TurnNumber {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].TurnNumber < reports[j].TurnNumber
	})
}

func sortChains(chains []Chain) {
	sort.Slice(chains, func(i, j int) bool {
		if chains[i].CreatedAtUS == chains[j].CreatedAtUS {
			return chains[i].ID < chains[j].ID
		}
		return chains[i].CreatedAtUS > chains[j].CreatedAtUS
	})
}

func sortChainSteps(steps []ChainStep) {
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].Sequence == steps[j].Sequence {
			return steps[i].ID < steps[j].ID
		}
		return steps[i].Sequence < steps[j].Sequence
	})
}

func sortChainEvents(events []ChainEvent) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].Sequence == events[j].Sequence {
			return events[i].ID < events[j].ID
		}
		return events[i].Sequence < events[j].Sequence
	})
}

func sortLaunchPresets(presets []LaunchPreset) {
	sort.Slice(presets, func(i, j int) bool {
		if presets[i].UpdatedAtUS == presets[j].UpdatedAtUS {
			return presets[i].Name < presets[j].Name
		}
		return presets[i].UpdatedAtUS > presets[j].UpdatedAtUS
	})
}

func conversationSearchScore(role string, titleMatch bool, contentMatch bool) int {
	score := 0
	if titleMatch {
		score += 40
	}
	if contentMatch {
		score += 10
	}
	switch role {
	case "assistant":
		score += 30
	case "user":
		score += 20
	case "tool":
		score += 10
	}
	return score
}

func filepathSlash(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
}

func normalizeKeyword(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func keywordScore(query string, doc Document, content string) float64 {
	score := 0.0
	if strings.Contains(normalizeKeyword(doc.Path), query) {
		score += 2
	}
	if strings.Contains(normalizeKeyword(doc.Title), query) {
		score += 2
	}
	if strings.Contains(normalizeKeyword(doc.TagsJSON), query) {
		score += 1
	}
	score += float64(strings.Count(normalizeKeyword(content), query))
	return score
}

func snippetAround(content string, normalizedQuery string) string {
	if content == "" {
		return ""
	}
	normalizedContent := normalizeKeyword(content)
	idx := strings.Index(normalizedContent, normalizedQuery)
	if idx < 0 {
		runes := []rune(content)
		if len(runes) > 160 {
			return string(runes[:160]) + "..."
		}
		return content
	}
	start := idx - 80
	if start < 0 {
		start = 0
	}
	end := idx + len(normalizedQuery) + 80
	if end > len(content) {
		end = len(content)
	}
	return strings.TrimSpace(content[start:end])
}
