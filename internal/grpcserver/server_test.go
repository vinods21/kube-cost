package grpcserver

import (
	"context"
	"testing"
)

func TestRunRequiresName(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), Config{}); err == nil {
		t.Fatal("expected missing service name to fail")
	}
}
