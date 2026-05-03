package tui

const footerHelp = "? help  / filter  ctrl+u clear  w web  tab screen  l launch  b preset  n add role  - remove role  s save  L load  v preview  S start  q quit"

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
