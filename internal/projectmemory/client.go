package projectmemory

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
)

type Client struct {
	rpc *rpc.Client
}

func DialBrainBackend(endpoint string) (*Client, error) {
	network, address, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, fmt.Errorf("dial project memory RPC endpoint %q: %w", endpoint, err)
	}
	return &Client{rpc: rpc.NewClient(conn)}, nil
}

func (c *Client) Close() error {
	if c == nil || c.rpc == nil {
		return nil
	}
	return c.rpc.Close()
}

func (c *Client) ReadDocument(ctx context.Context, path string) (string, error) {
	var resp ReadDocumentResponse
	if err := c.call(ctx, "Brain.ReadDocument", ReadDocumentRequest{Path: path}, &resp); err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (c *Client) WriteDocument(ctx context.Context, path string, content string) error {
	return c.call(ctx, "Brain.WriteDocument", WriteDocumentRequest{Path: path, Content: content}, &EmptyResponse{})
}

func (c *Client) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	return c.call(ctx, "Brain.PatchDocument", PatchDocumentRequest{Path: path, Operation: operation, Content: content}, &EmptyResponse{})
}

func (c *Client) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	return c.SearchKeywordLimit(ctx, query, 10)
}

func (c *Client) SearchKeywordLimit(ctx context.Context, query string, maxResults int) ([]brain.SearchHit, error) {
	var resp SearchKeywordResponse
	if err := c.call(ctx, "Brain.SearchKeywordLimit", SearchKeywordRequest{Query: query, MaxResults: maxResults}, &resp); err != nil {
		return nil, err
	}
	hits := make([]brain.SearchHit, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		hits = append(hits, brain.SearchHit{Path: hit.Path, Snippet: hit.Snippet, Score: hit.Score})
	}
	return hits, nil
}

func (c *Client) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	var resp ListDocumentsResponse
	if err := c.call(ctx, "Brain.ListDocuments", ListDocumentsRequest{Directory: directory}, &resp); err != nil {
		return nil, err
	}
	return resp.Paths, nil
}

func (c *Client) ReadBrainIndexState(ctx context.Context) (BrainIndexState, bool, error) {
	var resp ReadBrainIndexStateResponse
	if err := c.call(ctx, "Brain.ReadBrainIndexState", ReadBrainIndexStateRequest{ProjectID: DefaultProjectID}, &resp); err != nil {
		return BrainIndexState{}, false, err
	}
	return resp.State, resp.Found, nil
}

func (c *Client) MarkBrainIndexClean(ctx context.Context, indexedAt time.Time, metadataJSON string) error {
	return c.call(ctx, "Brain.MarkBrainIndexClean", MarkBrainIndexCleanRequest{
		ProjectID:       DefaultProjectID,
		LastIndexedAtUS: uint64(indexedAt.UTC().UnixMicro()),
		MetadataJSON:    metadataJSON,
	}, &EmptyResponse{})
}

func (c *Client) ReadCodeIndexState(ctx context.Context) (CodeIndexState, bool, error) {
	var resp ReadCodeIndexStateResponse
	if err := c.call(ctx, "Brain.ReadCodeIndexState", ReadCodeIndexStateRequest{ProjectID: DefaultProjectID}, &resp); err != nil {
		return CodeIndexState{}, false, err
	}
	return resp.State, resp.Found, nil
}

func (c *Client) ListCodeFileIndexStates(ctx context.Context) ([]CodeFileIndexState, error) {
	var resp ListCodeFileIndexStatesResponse
	if err := c.call(ctx, "Brain.ListCodeFileIndexStates", ListCodeFileIndexStatesRequest{ProjectID: DefaultProjectID}, &resp); err != nil {
		return nil, err
	}
	return resp.States, nil
}

func (c *Client) MarkCodeIndexClean(ctx context.Context, revision string, indexedAt time.Time, files []CodeFileIndexArg, deletedPaths []string, metadataJSON string) error {
	return c.call(ctx, "Brain.MarkCodeIndexClean", MarkCodeIndexCleanRequest{
		ProjectID:         DefaultProjectID,
		LastIndexedCommit: revision,
		LastIndexedAtUS:   uint64(indexedAt.UTC().UnixMicro()),
		Files:             files,
		DeletedPaths:      deletedPaths,
		MetadataJSON:      metadataJSON,
	}, &EmptyResponse{})
}

func (c *Client) CreateConversation(ctx context.Context, args CreateConversationArgs) error {
	return c.call(ctx, "Brain.CreateConversation", args, &EmptyResponse{})
}

func (c *Client) DeleteConversation(ctx context.Context, args DeleteConversationArgs) error {
	return c.call(ctx, "Brain.DeleteConversation", args, &EmptyResponse{})
}

func (c *Client) SetConversationTitle(ctx context.Context, args SetConversationTitleArgs) error {
	return c.call(ctx, "Brain.SetConversationTitle", args, &EmptyResponse{})
}

func (c *Client) SetRuntimeDefaults(ctx context.Context, args SetRuntimeDefaultsArgs) error {
	return c.call(ctx, "Brain.SetRuntimeDefaults", args, &EmptyResponse{})
}

func (c *Client) AppendUserMessage(ctx context.Context, args AppendUserMessageArgs) error {
	return c.call(ctx, "Brain.AppendUserMessage", args, &EmptyResponse{})
}

func (c *Client) PersistIteration(ctx context.Context, args PersistIterationArgs) error {
	return c.call(ctx, "Brain.PersistIteration", args, &EmptyResponse{})
}

func (c *Client) CompressMessages(ctx context.Context, args CompressMessagesArgs) error {
	return c.call(ctx, "Brain.CompressMessages", args, &EmptyResponse{})
}

func (c *Client) CancelIteration(ctx context.Context, args CancelIterationArgs) error {
	return c.call(ctx, "Brain.CancelIteration", args, &EmptyResponse{})
}

func (c *Client) DiscardTurn(ctx context.Context, args DiscardTurnArgs) error {
	return c.call(ctx, "Brain.DiscardTurn", args, &EmptyResponse{})
}

func (c *Client) ReadConversation(ctx context.Context, id string) (Conversation, bool, error) {
	var resp ReadConversationResponse
	if err := c.call(ctx, "Brain.ReadConversation", ReadConversationRequest{ID: id}, &resp); err != nil {
		return Conversation{}, false, err
	}
	return resp.Conversation, resp.Found, nil
}

func (c *Client) ListConversations(ctx context.Context, projectID string, limit, offset int) ([]Conversation, error) {
	var resp ListConversationsResponse
	if err := c.call(ctx, "Brain.ListConversations", ListConversationsRequest{ProjectID: projectID, Limit: limit, Offset: offset}, &resp); err != nil {
		return nil, err
	}
	return resp.Conversations, nil
}

func (c *Client) CountConversations(ctx context.Context, projectID string) (int64, error) {
	var resp CountConversationsResponse
	if err := c.call(ctx, "Brain.CountConversations", CountConversationsRequest{ProjectID: projectID}, &resp); err != nil {
		return 0, err
	}
	return resp.Count, nil
}

func (c *Client) ListMessages(ctx context.Context, conversationID string, includeCompressed bool) ([]Message, error) {
	var resp ListMessagesResponse
	if err := c.call(ctx, "Brain.ListMessages", ListMessagesRequest{ConversationID: conversationID, IncludeCompressed: includeCompressed}, &resp); err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

func (c *Client) GetMessagePage(ctx context.Context, conversationID string, limit, offset int) ([]Message, error) {
	var resp ListMessagesResponse
	if err := c.call(ctx, "Brain.GetMessagePage", GetMessagePageRequest{ConversationID: conversationID, Limit: limit, Offset: offset}, &resp); err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

func (c *Client) NextTurnNumber(ctx context.Context, conversationID string) (int, error) {
	var resp NextTurnNumberResponse
	if err := c.call(ctx, "Brain.NextTurnNumber", NextTurnNumberRequest{ConversationID: conversationID}, &resp); err != nil {
		return 0, err
	}
	return resp.TurnNumber, nil
}

func (c *Client) SearchConversations(ctx context.Context, projectID string, query string, maxResults int) ([]ConversationSearchHit, error) {
	var resp SearchConversationsResponse
	if err := c.call(ctx, "Brain.SearchConversations", SearchConversationsRequest{ProjectID: projectID, Query: query, MaxResults: maxResults}, &resp); err != nil {
		return nil, err
	}
	return resp.Hits, nil
}

func (c *Client) RecordSubCall(ctx context.Context, args RecordSubCallArgs) error {
	return c.call(ctx, "Brain.RecordSubCall", args, &EmptyResponse{})
}

func (c *Client) ListSubCalls(ctx context.Context, conversationID string) ([]SubCall, error) {
	var resp ListSubCallsResponse
	if err := c.call(ctx, "Brain.ListSubCalls", ListSubCallsRequest{ConversationID: conversationID}, &resp); err != nil {
		return nil, err
	}
	return resp.SubCalls, nil
}

func (c *Client) ListTurnSubCalls(ctx context.Context, conversationID string, turnNumber uint32) ([]SubCall, error) {
	var resp ListSubCallsResponse
	if err := c.call(ctx, "Brain.ListTurnSubCalls", ListTurnSubCallsRequest{ConversationID: conversationID, TurnNumber: turnNumber}, &resp); err != nil {
		return nil, err
	}
	return resp.SubCalls, nil
}

func (c *Client) RecordToolExecution(ctx context.Context, args RecordToolExecutionArgs) error {
	return c.call(ctx, "Brain.RecordToolExecution", args, &EmptyResponse{})
}

func (c *Client) ListToolExecutions(ctx context.Context, conversationID string) ([]ToolExecution, error) {
	var resp ListToolExecutionsResponse
	if err := c.call(ctx, "Brain.ListToolExecutions", ListToolExecutionsRequest{ConversationID: conversationID}, &resp); err != nil {
		return nil, err
	}
	return resp.Executions, nil
}

func (c *Client) ListTurnToolExecutions(ctx context.Context, conversationID string, turnNumber uint32) ([]ToolExecution, error) {
	var resp ListToolExecutionsResponse
	if err := c.call(ctx, "Brain.ListTurnToolExecutions", ListTurnToolExecutionsRequest{ConversationID: conversationID, TurnNumber: turnNumber}, &resp); err != nil {
		return nil, err
	}
	return resp.Executions, nil
}

func (c *Client) StoreContextReport(ctx context.Context, args StoreContextReportArgs) error {
	return c.call(ctx, "Brain.StoreContextReport", args, &EmptyResponse{})
}

func (c *Client) ReadContextReport(ctx context.Context, conversationID string, turnNumber uint32) (ContextReport, bool, error) {
	var resp ReadContextReportResponse
	if err := c.call(ctx, "Brain.ReadContextReport", ReadContextReportRequest{ConversationID: conversationID, TurnNumber: turnNumber}, &resp); err != nil {
		return ContextReport{}, false, err
	}
	return resp.Report, resp.Found, nil
}

func (c *Client) ListContextReports(ctx context.Context, conversationID string) ([]ContextReport, error) {
	var resp ListContextReportsResponse
	if err := c.call(ctx, "Brain.ListContextReports", ListContextReportsRequest{ConversationID: conversationID}, &resp); err != nil {
		return nil, err
	}
	return resp.Reports, nil
}

func (c *Client) UpdateContextReportQuality(ctx context.Context, args UpdateContextReportQualityArgs) error {
	return c.call(ctx, "Brain.UpdateContextReportQuality", args, &EmptyResponse{})
}

func (c *Client) StartChain(ctx context.Context, args StartChainArgs) error {
	return c.call(ctx, "Brain.StartChain", args, &EmptyResponse{})
}

func (c *Client) StartStep(ctx context.Context, args StartStepArgs) error {
	return c.call(ctx, "Brain.StartStep", args, &EmptyResponse{})
}

func (c *Client) StepRunning(ctx context.Context, args StepRunningArgs) error {
	return c.call(ctx, "Brain.StepRunning", args, &EmptyResponse{})
}

func (c *Client) CompleteStep(ctx context.Context, args CompleteStepArgs) error {
	return c.call(ctx, "Brain.CompleteStep", args, &EmptyResponse{})
}

func (c *Client) CompleteStepWithReceipt(ctx context.Context, args CompleteStepWithReceiptArgs) error {
	return c.call(ctx, "Brain.CompleteStepWithReceipt", args, &EmptyResponse{})
}

func (c *Client) CompleteChain(ctx context.Context, args CompleteChainArgs) error {
	return c.call(ctx, "Brain.CompleteChain", args, &EmptyResponse{})
}

func (c *Client) UpdateChainMetrics(ctx context.Context, args UpdateChainMetricsArgs) error {
	return c.call(ctx, "Brain.UpdateChainMetrics", args, &EmptyResponse{})
}

func (c *Client) SetChainStatus(ctx context.Context, args SetChainStatusArgs) error {
	return c.call(ctx, "Brain.SetChainStatus", args, &EmptyResponse{})
}

func (c *Client) LogChainEvent(ctx context.Context, args LogChainEventArgs) error {
	return c.call(ctx, "Brain.LogChainEvent", args, &EmptyResponse{})
}

func (c *Client) ReadChain(ctx context.Context, id string) (Chain, bool, error) {
	var resp ReadChainResponse
	if err := c.call(ctx, "Brain.ReadChain", ReadChainRequest{ID: id}, &resp); err != nil {
		return Chain{}, false, err
	}
	return resp.Chain, resp.Found, nil
}

func (c *Client) ListChains(ctx context.Context, limit int) ([]Chain, error) {
	var resp ListChainsResponse
	if err := c.call(ctx, "Brain.ListChains", ListChainsRequest{Limit: limit}, &resp); err != nil {
		return nil, err
	}
	return resp.Chains, nil
}

func (c *Client) ReadStep(ctx context.Context, id string) (ChainStep, bool, error) {
	var resp ReadStepResponse
	if err := c.call(ctx, "Brain.ReadStep", ReadStepRequest{ID: id}, &resp); err != nil {
		return ChainStep{}, false, err
	}
	return resp.Step, resp.Found, nil
}

func (c *Client) ListChainSteps(ctx context.Context, chainID string) ([]ChainStep, error) {
	var resp ListChainStepsResponse
	if err := c.call(ctx, "Brain.ListChainSteps", ListChainStepsRequest{ChainID: chainID}, &resp); err != nil {
		return nil, err
	}
	return resp.Steps, nil
}

func (c *Client) ListChainEvents(ctx context.Context, chainID string) ([]ChainEvent, error) {
	return c.ListChainEventsSince(ctx, chainID, 0)
}

func (c *Client) ListChainEventsSince(ctx context.Context, chainID string, afterSequence uint64) ([]ChainEvent, error) {
	var resp ListChainEventsResponse
	if err := c.call(ctx, "Brain.ListChainEventsSince", ListChainEventsSinceRequest{ChainID: chainID, AfterSequence: afterSequence}, &resp); err != nil {
		return nil, err
	}
	return resp.Events, nil
}

func (c *Client) SaveLaunch(ctx context.Context, args SaveLaunchArgs) error {
	return c.call(ctx, "Brain.SaveLaunch", args, &EmptyResponse{})
}

func (c *Client) ReadLaunch(ctx context.Context, projectID string, launchID string) (Launch, bool, error) {
	var resp ReadLaunchResponse
	if err := c.call(ctx, "Brain.ReadLaunch", ReadLaunchRequest{ProjectID: projectID, LaunchID: launchID}, &resp); err != nil {
		return Launch{}, false, err
	}
	return resp.Launch, resp.Found, nil
}

func (c *Client) SaveLaunchPreset(ctx context.Context, args SaveLaunchPresetArgs) error {
	return c.call(ctx, "Brain.SaveLaunchPreset", args, &EmptyResponse{})
}

func (c *Client) ListLaunchPresets(ctx context.Context, projectID string) ([]LaunchPreset, error) {
	var resp ListLaunchPresetsResponse
	if err := c.call(ctx, "Brain.ListLaunchPresets", ListLaunchPresetsRequest{ProjectID: projectID}, &resp); err != nil {
		return nil, err
	}
	return resp.Presets, nil
}

func (c *Client) call(ctx context.Context, method string, args any, reply any) error {
	if c == nil || c.rpc == nil {
		return fmt.Errorf("project memory RPC client is closed")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	call := c.rpc.Go(method, args, reply, make(chan *rpc.Call, 1))
	select {
	case done := <-call.Done:
		return done.Error
	case <-ctx.Done():
		return ctx.Err()
	}
}

func parseEndpoint(endpoint string) (string, string, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "", "", fmt.Errorf("project memory RPC endpoint is required")
	}
	transport, address, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", "", fmt.Errorf("project memory RPC endpoint %q must be transport:path", endpoint)
	}
	if transport != "unix" {
		return "", "", fmt.Errorf("unsupported project memory RPC transport %q", transport)
	}
	if strings.TrimSpace(address) == "" {
		return "", "", fmt.Errorf("project memory RPC endpoint path is required")
	}
	return "unix", address, nil
}
