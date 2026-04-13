package chain

import "fmt"

func NextControlStatus(currentStatus string, targetStatus string) (string, error) {
	switch targetStatus {
	case "paused":
		switch currentStatus {
		case "running", "pause_requested":
			return "pause_requested", nil
		case "paused":
			return "paused", nil
		default:
			return "", fmt.Errorf("chain is %s and cannot be paused", currentStatus)
		}
	case "cancelled":
		switch currentStatus {
		case "running", "pause_requested", "cancel_requested":
			return "cancel_requested", nil
		case "paused", "cancelled":
			return "cancelled", nil
		default:
			return "", fmt.Errorf("chain is %s and cannot be cancelled", currentStatus)
		}
	case "running":
		switch currentStatus {
		case "paused", "pause_requested", "running":
			return "running", nil
		default:
			return "", fmt.Errorf("chain is %s and cannot be resumed", currentStatus)
		}
	default:
		return "", fmt.Errorf("unsupported chain status transition to %s", targetStatus)
	}
}

func FinalizeControlStatus(status string) (string, bool) {
	switch status {
	case "pause_requested":
		return "paused", true
	case "cancel_requested":
		return "cancelled", true
	default:
		return "", false
	}
}

func ShouldStopScheduling(status string) bool {
	switch status {
	case "paused", "cancelled", "pause_requested", "cancel_requested":
		return true
	default:
		return false
	}
}
