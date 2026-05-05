package projectmemory

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type RPCConfig struct {
	Transport string
	Path      string
}

type RPCServer struct {
	listener net.Listener
	server   *rpc.Server
	backend  *BrainBackend
	path     string
	done     chan struct{}
	close    sync.Once
}

func StartRPCServer(ctx context.Context, cfg RPCConfig, backend *BrainBackend) (*RPCServer, error) {
	if backend == nil {
		return nil, fmt.Errorf("project memory RPC backend is required")
	}
	transport := strings.TrimSpace(cfg.Transport)
	if transport == "" {
		transport = "unix"
	}
	if transport != "unix" {
		return nil, fmt.Errorf("unsupported project memory RPC transport %q", cfg.Transport)
	}
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return nil, fmt.Errorf("project memory RPC path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir project memory RPC dir: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale project memory RPC socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen project memory RPC: %w", err)
	}
	srv := &RPCServer{
		listener: listener,
		server:   rpc.NewServer(),
		backend:  backend,
		path:     path,
		done:     make(chan struct{}),
	}
	if err := srv.server.RegisterName("Brain", &brainRPCService{backend: backend}); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("register project memory RPC service: %w", err)
	}
	go srv.serve()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
	}
	return srv, nil
}

func (s *RPCServer) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.close.Do(func() {
		err = s.listener.Close()
		<-s.done
		if s.path != "" {
			_ = os.Remove(s.path)
		}
	})
	return err
}

func (s *RPCServer) serve() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.server.ServeConn(conn)
	}
}

type brainRPCService struct {
	backend *BrainBackend
}

type ReadDocumentRequest struct {
	Path string
}

type ReadDocumentResponse struct {
	Content string
}

type WriteDocumentRequest struct {
	Path    string
	Content string
}

type PatchDocumentRequest struct {
	Path      string
	Operation string
	Content   string
}

type SearchKeywordRequest struct {
	Query      string
	MaxResults int
}

type SearchKeywordResponse struct {
	Hits []SearchHit
}

type ListDocumentsRequest struct {
	Directory string
}

type ListDocumentsResponse struct {
	Paths []string
}

type ReadBrainIndexStateRequest struct {
	ProjectID string
}

type ReadBrainIndexStateResponse struct {
	State BrainIndexState
	Found bool
}

type MarkBrainIndexCleanRequest struct {
	ProjectID       string
	LastIndexedAtUS uint64
	MetadataJSON    string
}

type ReadCodeIndexStateRequest struct {
	ProjectID string
}

type ReadCodeIndexStateResponse struct {
	State CodeIndexState
	Found bool
}

type ListCodeFileIndexStatesRequest struct {
	ProjectID string
}

type ListCodeFileIndexStatesResponse struct {
	States []CodeFileIndexState
}

type MarkCodeIndexCleanRequest struct {
	ProjectID         string
	LastIndexedCommit string
	LastIndexedAtUS   uint64
	Files             []CodeFileIndexArg
	DeletedPaths      []string
	MetadataJSON      string
}

type ReadConversationRequest struct {
	ID string
}

type ReadConversationResponse struct {
	Conversation Conversation
	Found        bool
}

type ListConversationsRequest struct {
	ProjectID string
	Limit     int
	Offset    int
}

type ListConversationsResponse struct {
	Conversations []Conversation
}

type CountConversationsRequest struct {
	ProjectID string
}

type CountConversationsResponse struct {
	Count int64
}

type ListMessagesRequest struct {
	ConversationID    string
	IncludeCompressed bool
}

type GetMessagePageRequest struct {
	ConversationID string
	Limit          int
	Offset         int
}

type ListMessagesResponse struct {
	Messages []Message
}

type NextTurnNumberRequest struct {
	ConversationID string
}

type NextTurnNumberResponse struct {
	TurnNumber int
}

type SearchConversationsRequest struct {
	ProjectID  string
	Query      string
	MaxResults int
}

type SearchConversationsResponse struct {
	Hits []ConversationSearchHit
}

type EmptyResponse struct{}

func (s *brainRPCService) ReadDocument(req ReadDocumentRequest, resp *ReadDocumentResponse) error {
	content, err := s.backend.ReadDocument(context.Background(), req.Path)
	if err != nil {
		return err
	}
	resp.Content = content
	return nil
}

func (s *brainRPCService) WriteDocument(req WriteDocumentRequest, resp *EmptyResponse) error {
	return s.backend.WriteDocument(context.Background(), req.Path, req.Content)
}

func (s *brainRPCService) PatchDocument(req PatchDocumentRequest, resp *EmptyResponse) error {
	return s.backend.PatchDocument(context.Background(), req.Path, req.Operation, req.Content)
}

func (s *brainRPCService) SearchKeywordLimit(req SearchKeywordRequest, resp *SearchKeywordResponse) error {
	hits, err := s.backend.runtime.SearchDocuments(context.Background(), req.Query, req.MaxResults)
	if err != nil {
		return err
	}
	resp.Hits = hits
	return nil
}

func (s *brainRPCService) ListDocuments(req ListDocumentsRequest, resp *ListDocumentsResponse) error {
	paths, err := s.backend.ListDocuments(context.Background(), req.Directory)
	if err != nil {
		return err
	}
	resp.Paths = paths
	return nil
}

func (s *brainRPCService) ReadBrainIndexState(req ReadBrainIndexStateRequest, resp *ReadBrainIndexStateResponse) error {
	state, found, err := s.backend.ReadBrainIndexState(context.Background())
	if err != nil {
		return err
	}
	resp.State = state
	resp.Found = found
	return nil
}

func (s *brainRPCService) MarkBrainIndexClean(req MarkBrainIndexCleanRequest, resp *EmptyResponse) error {
	return s.backend.runtime.MarkBrainIndexClean(context.Background(), MarkBrainIndexCleanArgs{
		ProjectID:       firstNonEmpty(req.ProjectID, DefaultProjectID),
		LastIndexedAtUS: req.LastIndexedAtUS,
		MetadataJSON:    req.MetadataJSON,
	})
}

func (s *brainRPCService) ReadCodeIndexState(req ReadCodeIndexStateRequest, resp *ReadCodeIndexStateResponse) error {
	state, found, err := s.backend.ReadCodeIndexState(context.Background())
	if err != nil {
		return err
	}
	resp.State = state
	resp.Found = found
	return nil
}

func (s *brainRPCService) ListCodeFileIndexStates(req ListCodeFileIndexStatesRequest, resp *ListCodeFileIndexStatesResponse) error {
	states, err := s.backend.ListCodeFileIndexStates(context.Background())
	if err != nil {
		return err
	}
	resp.States = states
	return nil
}

func (s *brainRPCService) MarkCodeIndexClean(req MarkCodeIndexCleanRequest, resp *EmptyResponse) error {
	return s.backend.runtime.MarkCodeIndexClean(context.Background(), MarkCodeIndexCleanArgs{
		ProjectID:         firstNonEmpty(req.ProjectID, DefaultProjectID),
		LastIndexedCommit: req.LastIndexedCommit,
		LastIndexedAtUS:   req.LastIndexedAtUS,
		Files:             req.Files,
		DeletedPaths:      req.DeletedPaths,
		MetadataJSON:      req.MetadataJSON,
	})
}

func (s *brainRPCService) CreateConversation(req CreateConversationArgs, resp *EmptyResponse) error {
	return s.backend.CreateConversation(context.Background(), req)
}

func (s *brainRPCService) DeleteConversation(req DeleteConversationArgs, resp *EmptyResponse) error {
	return s.backend.DeleteConversation(context.Background(), req)
}

func (s *brainRPCService) SetConversationTitle(req SetConversationTitleArgs, resp *EmptyResponse) error {
	return s.backend.SetConversationTitle(context.Background(), req)
}

func (s *brainRPCService) SetRuntimeDefaults(req SetRuntimeDefaultsArgs, resp *EmptyResponse) error {
	return s.backend.SetRuntimeDefaults(context.Background(), req)
}

func (s *brainRPCService) AppendUserMessage(req AppendUserMessageArgs, resp *EmptyResponse) error {
	return s.backend.AppendUserMessage(context.Background(), req)
}

func (s *brainRPCService) PersistIteration(req PersistIterationArgs, resp *EmptyResponse) error {
	return s.backend.PersistIteration(context.Background(), req)
}

func (s *brainRPCService) CancelIteration(req CancelIterationArgs, resp *EmptyResponse) error {
	return s.backend.CancelIteration(context.Background(), req)
}

func (s *brainRPCService) DiscardTurn(req DiscardTurnArgs, resp *EmptyResponse) error {
	return s.backend.DiscardTurn(context.Background(), req)
}

func (s *brainRPCService) ReadConversation(req ReadConversationRequest, resp *ReadConversationResponse) error {
	conversation, found, err := s.backend.ReadConversation(context.Background(), req.ID)
	if err != nil {
		return err
	}
	resp.Conversation = conversation
	resp.Found = found
	return nil
}

func (s *brainRPCService) ListConversations(req ListConversationsRequest, resp *ListConversationsResponse) error {
	conversations, err := s.backend.ListConversations(context.Background(), req.ProjectID, req.Limit, req.Offset)
	if err != nil {
		return err
	}
	resp.Conversations = conversations
	return nil
}

func (s *brainRPCService) CountConversations(req CountConversationsRequest, resp *CountConversationsResponse) error {
	count, err := s.backend.CountConversations(context.Background(), req.ProjectID)
	if err != nil {
		return err
	}
	resp.Count = count
	return nil
}

func (s *brainRPCService) ListMessages(req ListMessagesRequest, resp *ListMessagesResponse) error {
	messages, err := s.backend.ListMessages(context.Background(), req.ConversationID, req.IncludeCompressed)
	if err != nil {
		return err
	}
	resp.Messages = messages
	return nil
}

func (s *brainRPCService) GetMessagePage(req GetMessagePageRequest, resp *ListMessagesResponse) error {
	messages, err := s.backend.GetMessagePage(context.Background(), req.ConversationID, req.Limit, req.Offset)
	if err != nil {
		return err
	}
	resp.Messages = messages
	return nil
}

func (s *brainRPCService) NextTurnNumber(req NextTurnNumberRequest, resp *NextTurnNumberResponse) error {
	next, err := s.backend.NextTurnNumber(context.Background(), req.ConversationID)
	if err != nil {
		return err
	}
	resp.TurnNumber = next
	return nil
}

func (s *brainRPCService) SearchConversations(req SearchConversationsRequest, resp *SearchConversationsResponse) error {
	hits, err := s.backend.SearchConversations(context.Background(), req.ProjectID, req.Query, req.MaxResults)
	if err != nil {
		return err
	}
	resp.Hits = hits
	return nil
}
