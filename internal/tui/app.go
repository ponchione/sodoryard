package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

type Operator interface {
	RuntimeStatus(context.Context) (operator.RuntimeStatus, error)
	SetReasoningEffort(context.Context, string) (operator.RuntimeStatus, error)
	ListAgentRoles(context.Context) ([]operator.AgentRoleSummary, error)
	ListChains(context.Context, int) ([]operator.ChainSummary, error)
	GetChainDetail(context.Context, string) (operator.ChainDetail, error)
	ListEventsSince(context.Context, string, int64) ([]chain.Event, error)
	ReadReceipt(context.Context, string, string) (operator.ReceiptView, error)
	PauseChain(context.Context, string) (operator.ControlResult, error)
	ResumeChain(context.Context, string) (operator.ControlResult, error)
	CancelChain(context.Context, string) (operator.ControlResult, error)
	ValidateLaunch(context.Context, operator.LaunchRequest) (operator.LaunchPreview, error)
	SaveLaunchDraft(context.Context, operator.LaunchRequest) (operator.LaunchDraft, error)
	LoadLaunchDraft(context.Context) (operator.LaunchDraft, bool, error)
	ListLaunchPresets(context.Context) ([]operator.LaunchPreset, error)
	SaveLaunchPreset(context.Context, string, operator.LaunchRequest) (operator.LaunchPreset, error)
	StartChain(context.Context, operator.LaunchRequest) (operator.StartResult, error)
	SendChatMessage(context.Context, operator.ChatTurnRequest) (operator.ChatTurnResult, error)
}

type Options struct {
	Context         context.Context
	RefreshInterval time.Duration
	FollowInterval  time.Duration
	ChainLimit      int
	ReceiptOpener   ReceiptOpener
	WebBaseURL      string
}

func Run(ctx context.Context, svc Operator, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	opts.Context = ctx
	model := NewModel(svc, opts)
	_, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}

func NewModel(svc Operator, opts Options) Model {
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	if opts.RefreshInterval == 0 {
		opts.RefreshInterval = 5 * time.Second
	} else if opts.RefreshInterval < 0 {
		opts.RefreshInterval = 0
	}
	if opts.FollowInterval == 0 {
		opts.FollowInterval = time.Second
	} else if opts.FollowInterval < 0 {
		opts.FollowInterval = 0
	}
	if opts.ChainLimit <= 0 {
		opts.ChainLimit = 20
	}
	if opts.ReceiptOpener == nil {
		opts.ReceiptOpener = defaultReceiptOpener
	}
	model := Model{
		ctx:             opts.Context,
		svc:             svc,
		screen:          screenChat,
		width:           100,
		height:          30,
		refreshInterval: opts.RefreshInterval,
		followInterval:  opts.FollowInterval,
		chainLimit:      opts.ChainLimit,
		receiptOpener:   opts.ReceiptOpener,
		webBaseURLValue: opts.WebBaseURL,
		styles:          newStyles(),
	}
	model.chatComposer = newChatComposer(model.styles)
	model.consoleViewport = viewport.New(maxInt(24, model.contentWidth()-2), model.consoleViewportHeight())
	model.resizeChatComposer()
	model.resizeConsoleViewport()
	return model
}
