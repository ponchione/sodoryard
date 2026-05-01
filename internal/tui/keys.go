package tui

const footerHelp = "? help  tab screen  l launch  v preview  S start  F follow  P pause  X cancel  q quit"

func nextScreen(screen appScreen) appScreen {
	switch screen {
	case screenDashboard:
		return screenLaunch
	case screenLaunch:
		return screenChains
	case screenChains:
		return screenReceipts
	case screenReceipts:
		return screenDashboard
	default:
		return screenDashboard
	}
}
