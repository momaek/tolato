package infra

import "time"

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type FixedClock struct {
	Time time.Time
}

func (c FixedClock) Now() time.Time {
	return c.Time.UTC()
}
