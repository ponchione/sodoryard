package tui

import (
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

type dataLoadedMsg struct {
	Status          operator.RuntimeStatus
	Roles           []operator.AgentRoleSummary
	Chains          []operator.ChainSummary
	Detail          *operator.ChainDetail
	Receipt         *operator.ReceiptView
	SelectedChainID string
	Err             error
}

type controlMsg struct {
	Action string
	Result operator.ControlResult
	Err    error
}

type followEventsMsg struct {
	ChainID string
	Status  string
	Detail  *operator.ChainDetail
	Events  []chain.Event
	Err     error
}

type receiptOpenedMsg struct {
	Mode ReceiptOpenMode
	Path string
	Err  error
}

type launchPreviewMsg struct {
	Request operator.LaunchRequest
	Preview operator.LaunchPreview
	Err     error
}

type launchStartedMsg struct {
	Result operator.StartResult
	Err    error
}

type tickMsg time.Time

type followTickMsg time.Time
