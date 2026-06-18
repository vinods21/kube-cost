package transport

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/kube-cost/kube-cost/agent/inventory"
	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type ingestionTestServer struct {
	agentv1.UnimplementedAgentIngestionServiceServer
	received chan *agentv1.ObservationBatch
	hello    chan *agentv1.AgentHello
}

func (s *ingestionTestServer) Connect(stream grpc.BidiStreamingServer[agentv1.AgentToIngestion, agentv1.IngestionToAgent]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if first.GetHello() == nil {
		return context.Canceled
	}
	s.hello <- first.GetHello()
	if err := stream.Send(&agentv1.IngestionToAgent{
		Frame: &agentv1.IngestionToAgent_Hello{Hello: &agentv1.ServerHello{
			SessionId:               "test",
			SelectedProtocolVersion: &agentv1.ProtocolVersion{Major: 1},
		}},
	}); err != nil {
		return err
	}
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	batch := frame.GetBatch()
	s.received <- batch
	return stream.Send(&agentv1.IngestionToAgent{
		Frame: &agentv1.IngestionToAgent_Acknowledgement{Acknowledgement: &agentv1.BatchAcknowledgement{
			BatchId:                  batch.BatchId,
			ReceivedThroughSequence:  batch.LastSequence,
			PersistedThroughSequence: batch.LastSequence,
		}},
	})
}

func TestClientStreamsAndAcknowledgesInventory(t *testing.T) {
	t.Parallel()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	ingestion := &ingestionTestServer{
		received: make(chan *agentv1.ObservationBatch, 1),
		hello:    make(chan *agentv1.AgentHello, 1),
	}
	agentv1.RegisterAgentIngestionServiceServer(server, ingestion)
	go server.Serve(listener)
	t.Cleanup(server.Stop)

	buffer := NewBuffer(10)
	if err := buffer.Publish(context.Background(), inventory.Event{
		Key: "cluster/one",
		Observation: &agentv1.Observation{
			Payload: &agentv1.Observation_ClusterInventory{
				ClusterInventory: &agentv1.ClusterInventory{
					Record:     &agentv1.InventoryRecord{Operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT},
					ClusterUid: "one",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	client := NewClient(ClientConfig{
		TenantID:        "tenant",
		ClusterID:       "cluster",
		AgentInstanceID: "agent",
		AgentVersion:    "test",
		Endpoint:        "passthrough:///bufnet",
		BatchSize:       10,
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
	}, buffer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	select {
	case hello := <-ingestion.hello:
		var nodeMetrics, containerMetrics bool
		for _, capability := range hello.Capabilities {
			switch capability {
			case agentv1.Capability_CAPABILITY_GPU_METRICS:
				t.Fatalf("Agent V1 advertised unsupported capability %s", capability)
			case agentv1.Capability_CAPABILITY_NODE_METRICS:
				nodeMetrics = true
			case agentv1.Capability_CAPABILITY_CONTAINER_METRICS:
				containerMetrics = true
			}
		}
		if !nodeMetrics || !containerMetrics {
			t.Fatalf("agent metrics capabilities node=%t container=%t", nodeMetrics, containerMetrics)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for agent hello")
	}

	select {
	case batch := <-ingestion.received:
		if len(batch.Observations) != 1 || batch.Observations[0].GetClusterInventory() == nil {
			t.Fatalf("unexpected batch: %+v", batch)
		}
		if len(batch.PayloadChecksum) == 0 {
			t.Fatal("batch checksum is missing")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for streamed inventory")
	}

	deadline := time.Now().Add(2 * time.Second)
	for buffer.Len() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if buffer.Len() != 0 {
		t.Fatal("acknowledged event remained buffered")
	}
}
