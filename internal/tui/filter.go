package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ponchione/sodoryard/internal/operator"
)

func filterChains(chains []operator.ChainSummary, query string) []operator.ChainSummary {
	if filterQueryEmpty(query) {
		return append([]operator.ChainSummary(nil), chains...)
	}
	filtered := make([]operator.ChainSummary, 0, len(chains))
	for _, ch := range chains {
		if chainMatchesFilter(ch, query) {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}

func chainMatchesFilter(ch operator.ChainSummary, query string) bool {
	fields := []string{
		ch.ID,
		ch.Status,
		ch.SourceTask,
		strings.Join(ch.SourceSpecs, " "),
		strconv.Itoa(ch.TotalSteps),
		strconv.Itoa(ch.TotalTokens),
	}
	if ch.CurrentStep != nil {
		fields = append(fields,
			ch.CurrentStep.ID,
			strconv.Itoa(ch.CurrentStep.SequenceNum),
			ch.CurrentStep.Role,
			ch.CurrentStep.Status,
			ch.CurrentStep.Verdict,
			ch.CurrentStep.ReceiptPath,
			strconv.Itoa(ch.CurrentStep.TokensUsed),
		)
	}
	return textMatchesFilter(strings.Join(fields, " "), query)
}

func filterReceiptItems(items []receiptItem, loaded *operator.ReceiptView, query string) []receiptItem {
	if filterQueryEmpty(query) {
		return append([]receiptItem(nil), items...)
	}
	filtered := make([]receiptItem, 0, len(items))
	for _, item := range items {
		if receiptItemMatchesFilter(item, loaded, query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func receiptItemMatchesFilter(item receiptItem, loaded *operator.ReceiptView, query string) bool {
	fields := []string{item.Label, item.Step, item.Path}
	if receiptViewMatchesItem(loaded, item) {
		fields = append(fields, loaded.Content)
	}
	return textMatchesFilter(strings.Join(fields, " "), query)
}

func receiptViewMatchesItem(receipt *operator.ReceiptView, item receiptItem) bool {
	if receipt == nil {
		return false
	}
	if receipt.Path != "" && receipt.Path == item.Path {
		return true
	}
	return receipt.Step != "" && receipt.Step == item.Step
}

func textMatchesFilter(text string, query string) bool {
	text = strings.ToLower(text)
	for _, part := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func filterQueryEmpty(query string) bool {
	return strings.TrimSpace(query) == ""
}

func (m Model) visibleChains() []operator.ChainSummary {
	return filterChains(m.chains, m.chainFilter)
}

func (m Model) visibleReceiptItems() []receiptItem {
	return filterReceiptItems(m.receiptItems, m.receipt, m.receiptFilter)
}

func (m Model) selectedVisibleChainID() string {
	return selectedChainID(m.visibleChains(), m.chainCursor)
}

func (m Model) selectedVisibleReceiptItem() (receiptItem, bool) {
	items := m.visibleReceiptItems()
	if len(items) == 0 {
		return receiptItem{}, false
	}
	return items[clampCursor(m.receiptCursor, len(items))], true
}

func (m Model) currentFilterText() string {
	switch m.filterScreen {
	case screenReceipts:
		return m.receiptFilter
	default:
		return m.chainFilter
	}
}

func (m *Model) setCurrentFilterText(value string) {
	switch m.filterScreen {
	case screenReceipts:
		m.receiptFilter = value
	default:
		m.chainFilter = value
	}
}

func (m Model) filterLabel() string {
	switch m.filterScreen {
	case screenReceipts:
		return "receipt"
	default:
		return "chain"
	}
}

func (m Model) filterAvailable() bool {
	return m.screen == screenChains || m.screen == screenReceipts
}

func renderFilterLine(label string, query string, editing bool, visible int, total int) string {
	query = strings.TrimSpace(query)
	if query == "" && !editing {
		return ""
	}
	value := query
	if value == "" {
		value = "empty"
	}
	suffix := ""
	if editing {
		suffix = " _"
	}
	return fmt.Sprintf("filter: %s%s (%d/%d %s)", value, suffix, visible, total, label)
}

func (m *Model) syncFilteredChainSelection(previousID string) {
	visible := m.visibleChains()
	m.chainCursor = chainIndexByID(visible, previousID, m.chainCursor)
	selectedID := selectedChainID(visible, m.chainCursor)
	if selectedID == "" || m.detail == nil || m.detail.Chain.ID != selectedID {
		m.detail = nil
		m.receiptItems = nil
		m.receipt = nil
		m.receiptCursor = 0
		m.updateReceiptViewport()
	}
}

func (m *Model) syncFilteredReceiptSelection(previous receiptItem, hadPrevious bool) {
	visible := m.visibleReceiptItems()
	m.receiptCursor = receiptIndexByItem(visible, previous, hadPrevious, m.receiptCursor)
	item, ok := m.selectedVisibleReceiptItem()
	if !ok || !receiptViewMatchesItem(m.receipt, item) {
		m.receipt = nil
		m.updateReceiptViewport()
	}
}

func receiptIndexByItem(items []receiptItem, item receiptItem, hasItem bool, fallback int) int {
	if hasItem {
		for i, candidate := range items {
			if candidate.Step == item.Step && candidate.Path == item.Path && candidate.Label == item.Label {
				return i
			}
		}
	}
	return clampCursor(fallback, len(items))
}
