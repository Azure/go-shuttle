package shuttle

import "time"

// RetryDelayStrategy can be implemented to provide custom delay retry strategies.
type RetryDelayStrategy interface {
	GetDelay(attempt uint32) time.Duration
}

// ConstantDelayStrategy delays the message retry by the given duration
type ConstantDelayStrategy struct {
	Delay time.Duration
}

func (s *ConstantDelayStrategy) GetDelay(_ uint32) time.Duration {
	return s.Delay
}
