package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
)

type Event struct {
	Key         string
	Observation *agentv1.Observation
}

func (e Event) Validate() error {
	if e.Key == "" {
		return fmt.Errorf("inventory event key is required")
	}
	if e.Observation == nil || e.Observation.Payload == nil {
		return fmt.Errorf("inventory event payload is required")
	}
	return nil
}

func Fingerprint(observation *agentv1.Observation) (string, error) {
	if observation == nil || observation.Payload == nil {
		return "", fmt.Errorf("observation payload is required")
	}
	clone := proto.Clone(observation).(*agentv1.Observation)
	clone.Sequence = 0
	clone.EventId = ""
	clone.ObservedAt = nil
	clone.CollectedAt = nil
	clone.SourceResourceVersion = ""
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("marshal observation: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
