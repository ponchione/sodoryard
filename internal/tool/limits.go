package tool

func boundedPositiveInt(value *int, fallback, max int) int {
	if max > 0 && fallback > max {
		fallback = max
	}
	if value == nil || *value <= 0 {
		return fallback
	}
	if max > 0 && *value > max {
		return max
	}
	return *value
}

func boundedNonNegativeInt(value *int, fallback, max int) int {
	if max > 0 && fallback > max {
		fallback = max
	}
	if value == nil {
		return fallback
	}
	if *value <= 0 {
		return 0
	}
	if max > 0 && *value > max {
		return max
	}
	return *value
}
