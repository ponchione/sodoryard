package tui

const footerHelp = "? help  enter send/edit  alt+enter newline  a chat  / filter  w web  tab screen  l launch  v preview  S start  q quit"

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
