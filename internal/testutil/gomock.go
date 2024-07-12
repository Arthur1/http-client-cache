package testutil

import (
	"log"
	"testing"
)

// https://github.com/golang/mock/issues/145
type ConcurrentTestReporter struct {
	*testing.T
}

func NewConcurrentTestReporter(t *testing.T) *ConcurrentTestReporter {
	return &ConcurrentTestReporter{t}
}

func (r *ConcurrentTestReporter) Fatalf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}
