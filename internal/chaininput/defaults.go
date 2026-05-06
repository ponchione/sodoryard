package chaininput

import "time"

const (
	DefaultMaxSteps         = 100
	DefaultMaxResolverLoops = 3
	DefaultMaxDuration      = 4 * time.Hour
	DefaultTokenBudget      = 5_000_000
)

type Limits struct {
	MaxSteps         int
	MaxResolverLoops int
	MaxDuration      time.Duration
	TokenBudget      int
}

func DefaultLimits() Limits {
	return Limits{
		MaxSteps:         DefaultMaxSteps,
		MaxResolverLoops: DefaultMaxResolverLoops,
		MaxDuration:      DefaultMaxDuration,
		TokenBudget:      DefaultTokenBudget,
	}
}

func NormalizeLimits(limits Limits) Limits {
	defaults := DefaultLimits()
	if limits.MaxSteps <= 0 {
		limits.MaxSteps = defaults.MaxSteps
	}
	if limits.MaxResolverLoops <= 0 {
		limits.MaxResolverLoops = defaults.MaxResolverLoops
	}
	if limits.MaxDuration <= 0 {
		limits.MaxDuration = defaults.MaxDuration
	}
	if limits.TokenBudget <= 0 {
		limits.TokenBudget = defaults.TokenBudget
	}
	return limits
}
