package tui

func (m Model) footerHelp() string {
	switch m.screen {
	case screenChat:
		if m.chatEdit {
			return "/help commands  /new clear  enter send/run  pgup/pgdn scroll  ctrl+u clear  ctrl+c quit"
		}
		if m.chatRunning {
			return "/help commands  ctrl+g cancel  pgup/pgdn scroll  /new clear  ctrl+c quit"
		}
		return "/ starts command  enter/i edit  pgup/pgdn scroll  N new session  ctrl+c quit"
	case screenDashboard:
		return "? help  enter chains  r refresh  w web  tab screen  q quit"
	case screenLaunch:
		return "? help  i edit  b preset  m mode  n add role  v preview  S start  q quit"
	case screenChains:
		return "? help  enter receipts  F follow  P pause  X cancel  / filter  tab screen  q quit"
	case screenReceipts:
		return "? help  o pager  E editor  esc chains  / filter  tab screen  q quit"
	case screenHelp:
		return "? close help  tab previous  q quit"
	default:
		return "? help  tab screen  q quit"
	}
}

func nextScreen(screen appScreen) appScreen {
	switch screen {
	case screenChat:
		return screenDashboard
	case screenDashboard:
		return screenLaunch
	case screenLaunch:
		return screenChains
	case screenChains:
		return screenReceipts
	case screenReceipts:
		return screenChat
	default:
		return screenChat
	}
}
