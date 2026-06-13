package app

import "testing"

func TestRuntimeRequiresLeaderElection(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{}
	if !runtime.NeedLeaderElection() {
		t.Fatal("agent runtime must require leader election")
	}
}
