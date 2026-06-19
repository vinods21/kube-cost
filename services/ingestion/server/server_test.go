package server

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"github.com/kube-cost/kube-cost/services/ingestion/queue"
	"google.golang.org/grpc/metadata"
)

func TestConnectAcknowledgesAndQueuesBatch(t *testing.T) {
	t.Parallel()
	batches := queue.New(10)
	persisted := startQueueCommitter(t, batches, 1)
	stream := runStream(t, New(testConfig(), batches, InsecureAuthenticator{}), batch(1, 2))

	ack := stream.sent[1].GetAcknowledgement()
	if ack.GetPersistedThroughSequence() != 2 || len(ack.GetRetryRanges()) != 0 {
		t.Fatalf("acknowledgement = %#v, want persisted through 2", ack)
	}
	queued := <-persisted
	if queued[0].TenantID != "tenant-a" || queued[0].ObservationSet.GetFirstSequence() != 1 {
		t.Fatalf("queued batch = %#v", queued[0])
	}
}

func TestConnectHandlesDuplicateAndPartialRetry(t *testing.T) {
	t.Parallel()
	batches := queue.New(10)
	persisted := startQueueCommitter(t, batches, 2)
	stream := runStream(t, New(testConfig(), batches, InsecureAuthenticator{}),
		batch(1, 2),
		batch(1, 2),
		batch(2, 3),
	)

	if got := len(stream.sent); got != 4 {
		t.Fatalf("sent frame count = %d, want 4", got)
	}
	if got := stream.sent[3].GetAcknowledgement().GetPersistedThroughSequence(); got != 3 {
		t.Fatalf("partial retry persisted through = %d, want 3", got)
	}
	first := <-persisted
	second := <-persisted
	queued := append(first, second...)
	if len(queued) != 2 {
		t.Fatalf("queued batch count = %d, want 2", len(queued))
	}
	if queued[1].ObservationSet.GetFirstSequence() != 3 || len(queued[1].ObservationSet.GetObservations()) != 1 {
		t.Fatalf("partial retry queued batch = %#v, want sequence 3 only", queued[1].ObservationSet)
	}
}

func TestConnectRequestsMissingSequenceRange(t *testing.T) {
	t.Parallel()
	stream := runStream(t, New(testConfig(), queue.New(10), InsecureAuthenticator{}), batch(3, 3))
	ack := stream.sent[1].GetAcknowledgement()
	if len(ack.GetRetryRanges()) != 1 ||
		ack.GetRetryRanges()[0].GetFirstSequence() != 1 ||
		ack.GetRetryRanges()[0].GetLastSequence() != 2 {
		t.Fatalf("retry ranges = %#v, want 1-2", ack.GetRetryRanges())
	}
}

func TestConnectAppliesBackpressureWhenQueueIsFull(t *testing.T) {
	t.Parallel()
	batches := queue.New(1)
	if err := batches.TryEnqueue(&queue.Batch{TenantID: "existing"}); err != nil {
		t.Fatal(err)
	}
	stream := runStream(t, New(testConfig(), batches, InsecureAuthenticator{}), batch(1, 1))
	if got := stream.sent[1].GetFlowControl(); got == nil || got.GetRetryAfter().AsDuration() <= 0 {
		t.Fatalf("flow control = %#v, want retry delay", got)
	}
	ack := stream.sent[2].GetAcknowledgement()
	if ack.GetPersistedThroughSequence() != 0 ||
		len(ack.GetRetryRanges()) != 1 ||
		ack.GetRetryRanges()[0].GetFirstSequence() != 1 {
		t.Fatalf("backpressure acknowledgement = %#v", ack)
	}
}

func TestConnectWaitsForQueueCommitBeforePersistedAck(t *testing.T) {
	t.Parallel()
	batches := queue.New(10)
	release := make(chan struct{})
	workerReady := make(chan struct{})
	go func() {
		lease, err := batches.Acquire(context.Background(), 1)
		if err != nil {
			return
		}
		close(workerReady)
		<-release
		lease.Commit()
	}()

	streamDone := make(chan *fakeStream, 1)
	go func() {
		streamDone <- runStream(t, New(testConfig(), batches, InsecureAuthenticator{}), batch(1, 1))
	}()

	select {
	case <-workerReady:
	case <-time.After(time.Second):
		t.Fatal("batch was not acquired by persistence worker")
	}
	select {
	case stream := <-streamDone:
		t.Fatalf("stream completed before queue commit: %#v", stream.sent)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	stream := <-streamDone
	if got := stream.sent[1].GetAcknowledgement().GetPersistedThroughSequence(); got != 1 {
		t.Fatalf("persisted through = %d, want 1", got)
	}
}

func TestConnectRejectsInvalidChecksumWithoutAdvancing(t *testing.T) {
	t.Parallel()
	invalid := batch(1, 1)
	invalid.PayloadChecksum = []byte("invalid")
	stream := runStream(t, New(testConfig(), queue.New(10), InsecureAuthenticator{}), invalid)
	ack := stream.sent[1].GetAcknowledgement()
	if ack.GetPersistedThroughSequence() != 0 || len(ack.GetRejections()) != 1 || ack.GetRejections()[0].GetRetryable() {
		t.Fatalf("invalid batch acknowledgement = %#v", ack)
	}
}

func TestConnectRetriesWhenArchiveFails(t *testing.T) {
	t.Parallel()
	stream := runStream(t, New(testConfig(), queue.New(10), InsecureAuthenticator{}, failingArchiver{}), batch(1, 1))
	ack := stream.sent[1].GetAcknowledgement()
	if ack.GetPersistedThroughSequence() != 0 ||
		len(ack.GetRetryRanges()) != 1 ||
		ack.GetRetryRanges()[0].GetFirstSequence() != 1 {
		t.Fatalf("archive failure acknowledgement = %#v", ack)
	}
}

func TestConnectArchivesAcceptedSuffix(t *testing.T) {
	t.Parallel()
	batches := queue.New(10)
	persisted := startQueueCommitter(t, batches, 2)
	archiver := &recordingArchiver{}
	runStream(t, New(testConfig(), batches, InsecureAuthenticator{}, archiver), batch(1, 2), batch(2, 3))
	<-persisted
	<-persisted
	if len(archiver.records) != 2 {
		t.Fatalf("archive records=%d, want 2", len(archiver.records))
	}
	if archiver.records[1].Batch.GetFirstSequence() != 3 || archiver.records[1].Batch.GetLastSequence() != 3 {
		t.Fatalf("archived suffix = %#v", archiver.records[1].Batch)
	}
}

func runStream(t *testing.T, server *Server, batches ...*agentv1.ObservationBatch) *fakeStream {
	t.Helper()
	frames := []*agentv1.AgentToIngestion{{
		Frame: &agentv1.AgentToIngestion_Hello{Hello: &agentv1.AgentHello{
			TenantId:        "tenant-a",
			ClusterId:       "cluster-a",
			AgentInstanceId: "agent-a",
			SupportedProtocolVersions: []*agentv1.ProtocolVersion{
				{Major: 1},
			},
		}},
	}}
	for _, item := range batches {
		frames = append(frames, &agentv1.AgentToIngestion{
			Frame: &agentv1.AgentToIngestion_Batch{Batch: item},
		})
	}
	stream := &fakeStream{ctx: context.Background(), received: frames}
	if err := server.Connect(stream); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return stream
}

func startQueueCommitter(t *testing.T, batches *queue.Queue, count int) <-chan []*queue.Batch {
	t.Helper()
	persisted := make(chan []*queue.Batch, count)
	go func() {
		for index := 0; index < count; index++ {
			lease, err := batches.Acquire(context.Background(), 1)
			if err != nil {
				return
			}
			items := lease.Items()
			lease.Commit()
			persisted <- items
		}
	}()
	return persisted
}

func batch(first, last uint64) *agentv1.ObservationBatch {
	observations := make([]*agentv1.Observation, 0, last-first+1)
	for sequence := first; sequence <= last; sequence++ {
		observations = append(observations, &agentv1.Observation{
			Sequence: sequence,
			EventId:  "event-" + time.Unix(int64(sequence), 0).UTC().Format(time.RFC3339),
			Payload: &agentv1.Observation_ClusterInventory{
				ClusterInventory: &agentv1.ClusterInventory{
					Record: &agentv1.InventoryRecord{
						Operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT,
					},
				},
			},
		})
	}
	return &agentv1.ObservationBatch{
		BatchId:         "batch-" + time.Unix(int64(first), int64(last)).UTC().Format(time.RFC3339Nano),
		FirstSequence:   first,
		LastSequence:    last,
		Observations:    observations,
		PayloadChecksum: batchChecksum(observations),
	}
}

func testConfig() Config {
	return Config{
		MaxBatchRecords:      10,
		MaxBatchBytes:        1 << 20,
		HeartbeatInterval:    30 * time.Second,
		BackpressureDelay:    time.Millisecond,
		HighWatermarkPercent: 80,
	}
}

type fakeStream struct {
	ctx      context.Context
	received []*agentv1.AgentToIngestion
	sent     []*agentv1.IngestionToAgent
}

func (s *fakeStream) Send(frame *agentv1.IngestionToAgent) error {
	s.sent = append(s.sent, frame)
	return nil
}

func (s *fakeStream) Recv() (*agentv1.AgentToIngestion, error) {
	if len(s.received) == 0 {
		return nil, io.EOF
	}
	frame := s.received[0]
	s.received = s.received[1:]
	return frame, nil
}

func (s *fakeStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeStream) SetTrailer(metadata.MD)       {}
func (s *fakeStream) Context() context.Context     { return s.ctx }
func (s *fakeStream) SendMsg(any) error            { return nil }
func (s *fakeStream) RecvMsg(any) error            { return nil }

type failingArchiver struct{}

func (failingArchiver) Archive(context.Context, ArchiveRecord) error {
	return errors.New("archive down")
}

type recordingArchiver struct {
	records []ArchiveRecord
}

func (a *recordingArchiver) Archive(_ context.Context, record ArchiveRecord) error {
	a.records = append(a.records, record)
	return nil
}
