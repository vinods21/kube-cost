package server

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"github.com/kube-cost/kube-cost/services/ingestion/queue"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const protocolMajor uint32 = 1

type Config struct {
	MaxBatchRecords      uint32
	MaxBatchBytes        uint64
	HeartbeatInterval    time.Duration
	BackpressureDelay    time.Duration
	HighWatermarkPercent int
}

type Server struct {
	agentv1.UnimplementedAgentIngestionServiceServer

	config        Config
	queue         *queue.Queue
	authenticator Authenticator

	mu       sync.Mutex
	clusters map[string]*clusterState
	active   map[string]string
}

type clusterState struct {
	mu               sync.Mutex
	persistedThrough uint64
}

func New(config Config, batches *queue.Queue, authenticator Authenticator) *Server {
	if config.MaxBatchRecords == 0 {
		config.MaxBatchRecords = 500
	}
	if config.MaxBatchBytes == 0 {
		config.MaxBatchBytes = 4 << 20
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.BackpressureDelay <= 0 {
		config.BackpressureDelay = time.Second
	}
	if config.HighWatermarkPercent <= 0 || config.HighWatermarkPercent > 100 {
		config.HighWatermarkPercent = 80
	}
	return &Server{
		config:        config,
		queue:         batches,
		authenticator: authenticator,
		clusters:      make(map[string]*clusterState),
		active:        make(map[string]string),
	}
}

func (s *Server) Connect(stream grpc.BidiStreamingServer[agentv1.AgentToIngestion, agentv1.IngestionToAgent]) error {
	first, err := stream.Recv()
	if err != nil {
		return receiveError(err)
	}
	hello := first.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first stream frame must be AgentHello")
	}
	if err := validateHello(hello); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.authenticator.Authenticate(stream.Context(), hello); err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	if !supportsProtocol(hello.GetSupportedProtocolVersions()) {
		return status.Error(codes.FailedPrecondition, "agent does not support protocol major version 1")
	}

	sessionID := uuid.NewString()
	clusterKey := hello.GetTenantId() + "\x00" + hello.GetClusterId()
	persistedThrough, acquired := s.acquire(clusterKey, sessionID)
	if !acquired {
		return status.Error(codes.AlreadyExists, "another stream is active for this tenant and cluster")
	}
	defer s.release(clusterKey, sessionID)

	if err := stream.Send(serverHello(sessionID, persistedThrough, s.config)); err != nil {
		return status.Errorf(codes.Unavailable, "send server hello: %v", err)
	}

	for {
		frame, err := stream.Recv()
		if err != nil {
			return receiveError(err)
		}
		switch {
		case frame.GetBatch() != nil:
			if err := s.handleBatch(stream, clusterKey, sessionID, hello, frame.GetBatch()); err != nil {
				return err
			}
		case frame.GetHeartbeat() != nil:
			continue
		case frame.GetHello() != nil:
			return status.Error(codes.InvalidArgument, "AgentHello may only be sent as the first frame")
		default:
			return status.Error(codes.InvalidArgument, "stream frame has no supported payload")
		}
	}
}

func (s *Server) handleBatch(
	stream grpc.BidiStreamingServer[agentv1.AgentToIngestion, agentv1.IngestionToAgent],
	clusterKey string,
	sessionID string,
	hello *agentv1.AgentHello,
	batch *agentv1.ObservationBatch,
) error {
	if err := validateBatch(batch, s.config); err != nil {
		persisted := s.persisted(clusterKey)
		return stream.Send(acknowledgement(
			batch.GetBatchId(),
			persisted,
			persisted,
			nil,
			rejectionsForBatch(batch, rejectionCode(err), err.Error()),
		))
	}

	s.mu.Lock()
	state := s.clusters[clusterKey]
	s.mu.Unlock()
	state.mu.Lock()
	defer state.mu.Unlock()
	persisted := state.persistedThrough

	if batch.GetLastSequence() <= persisted {
		return stream.Send(acknowledgement(batch.GetBatchId(), batch.GetLastSequence(), persisted, nil, nil))
	}
	if batch.GetFirstSequence() > persisted+1 {
		retry := []*agentv1.SequenceRange{{
			FirstSequence: persisted + 1,
			LastSequence:  batch.GetFirstSequence() - 1,
		}}
		return stream.Send(acknowledgement(batch.GetBatchId(), batch.GetLastSequence(), persisted, retry, nil))
	}

	accepted := suffixAfter(batch, persisted)
	flowControlSent := false
	if s.queue.Depth() >= s.highWatermark() {
		if err := stream.Send(flowControl(s.config.BackpressureDelay, "ingestion queue is above its high watermark")); err != nil {
			return err
		}
		flowControlSent = true
	}
	if err := s.queue.TryEnqueue(&queue.Batch{
		TenantID:       hello.GetTenantId(),
		ClusterID:      hello.GetClusterId(),
		AgentInstance:  hello.GetAgentInstanceId(),
		SessionID:      sessionID,
		ReceivedAt:     time.Now().UTC(),
		ObservationSet: accepted,
	}); err != nil {
		if !errors.Is(err, queue.ErrFull) {
			return status.Errorf(codes.Internal, "enqueue batch: %v", err)
		}
		if !flowControlSent {
			if sendErr := stream.Send(flowControl(s.config.BackpressureDelay, "ingestion queue is full")); sendErr != nil {
				return sendErr
			}
		}
		retry := []*agentv1.SequenceRange{{
			FirstSequence: accepted.GetFirstSequence(),
			LastSequence:  accepted.GetLastSequence(),
		}}
		return stream.Send(acknowledgement(batch.GetBatchId(), batch.GetLastSequence(), persisted, retry, nil))
	}

	state.persistedThrough = batch.GetLastSequence()
	return stream.Send(acknowledgement(
		batch.GetBatchId(),
		batch.GetLastSequence(),
		state.persistedThrough,
		nil,
		nil,
	))
}

func (s *Server) acquire(clusterKey, sessionID string) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.active[clusterKey]; exists {
		return 0, false
	}
	state := s.clusters[clusterKey]
	if state == nil {
		state = &clusterState{}
		s.clusters[clusterKey] = state
	}
	s.active[clusterKey] = sessionID
	return state.persistedThrough, true
}

func (s *Server) release(clusterKey, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[clusterKey] == sessionID {
		delete(s.active, clusterKey)
	}
}

func (s *Server) persisted(clusterKey string) uint64 {
	s.mu.Lock()
	state := s.clusters[clusterKey]
	s.mu.Unlock()
	if state == nil {
		return 0
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.persistedThrough
}

func (s *Server) highWatermark() int {
	return max(1, s.queue.Capacity()*s.config.HighWatermarkPercent/100)
}

func validateHello(hello *agentv1.AgentHello) error {
	if hello.GetTenantId() == "" {
		return errors.New("tenant_id is required")
	}
	if hello.GetClusterId() == "" {
		return errors.New("cluster_id is required")
	}
	if hello.GetAgentInstanceId() == "" {
		return errors.New("agent_instance_id is required")
	}
	return nil
}

func validateBatch(batch *agentv1.ObservationBatch, config Config) error {
	if batch == nil || batch.GetBatchId() == "" {
		return errors.New("batch_id is required")
	}
	if len(batch.GetObservations()) == 0 {
		return errors.New("batch must contain observations")
	}
	if uint32(len(batch.GetObservations())) > config.MaxBatchRecords {
		return fmt.Errorf("batch contains %d records; maximum is %d", len(batch.GetObservations()), config.MaxBatchRecords)
	}
	if uint64(proto.Size(batch)) > config.MaxBatchBytes {
		return fmt.Errorf("batch size exceeds maximum of %d bytes", config.MaxBatchBytes)
	}
	if batch.GetFirstSequence() == 0 || batch.GetLastSequence() < batch.GetFirstSequence() {
		return errors.New("batch sequence range is invalid")
	}
	if uint64(len(batch.GetObservations())) != batch.GetLastSequence()-batch.GetFirstSequence()+1 {
		return errors.New("batch sequence range does not match observation count")
	}
	for index, observation := range batch.GetObservations() {
		expected := batch.GetFirstSequence() + uint64(index)
		if observation.GetSequence() != expected {
			return fmt.Errorf("observation sequence %d is not expected sequence %d", observation.GetSequence(), expected)
		}
		if observation.GetEventId() == "" {
			return fmt.Errorf("observation %d has no event_id", expected)
		}
		if observation.GetPayload() == nil {
			return fmt.Errorf("observation %d has no payload", expected)
		}
		if err := validateInventoryObservation(observation); err != nil {
			return fmt.Errorf("observation %d: %w", expected, err)
		}
	}
	if !bytes.Equal(batch.GetPayloadChecksum(), batchChecksum(batch.GetObservations())) {
		return errors.New("payload checksum does not match observations")
	}
	return nil
}

func validateInventoryObservation(observation *agentv1.Observation) error {
	switch payload := observation.GetPayload().(type) {
	case *agentv1.Observation_ClusterInventory:
		return validateInventoryRecord(payload.ClusterInventory.GetRecord())
	case *agentv1.Observation_NodeInventory:
		if err := validateInventoryRecord(payload.NodeInventory.GetRecord()); err != nil {
			return err
		}
		if payload.NodeInventory.GetMetadata().GetUid() == "" {
			return errors.New("node UID is required")
		}
	case *agentv1.Observation_NamespaceInventory:
		if err := validateInventoryRecord(payload.NamespaceInventory.GetRecord()); err != nil {
			return err
		}
		if payload.NamespaceInventory.GetMetadata().GetUid() == "" {
			return errors.New("namespace UID is required")
		}
	case *agentv1.Observation_DeploymentInventory:
		if err := validateInventoryRecord(payload.DeploymentInventory.GetRecord()); err != nil {
			return err
		}
		if payload.DeploymentInventory.GetMetadata().GetUid() == "" {
			return errors.New("deployment UID is required")
		}
	case *agentv1.Observation_PodInventory:
		if err := validateInventoryRecord(payload.PodInventory.GetRecord()); err != nil {
			return err
		}
		if payload.PodInventory.GetMetadata().GetUid() == "" {
			return errors.New("pod UID is required")
		}
	case *agentv1.Observation_ContainerInventory:
		if err := validateInventoryRecord(payload.ContainerInventory.GetRecord()); err != nil {
			return err
		}
		if payload.ContainerInventory.GetPodUid() == "" || payload.ContainerInventory.GetContainerName() == "" {
			return errors.New("container pod UID and name are required")
		}
	}
	return nil
}

func validateInventoryRecord(record *agentv1.InventoryRecord) error {
	switch record.GetOperation() {
	case agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT,
		agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE:
		return nil
	default:
		return errors.New("inventory operation is required")
	}
}

func batchChecksum(observations []*agentv1.Observation) []byte {
	hash := sha256.New()
	for _, observation := range observations {
		data, err := proto.MarshalOptions{Deterministic: true}.Marshal(observation)
		if err != nil {
			continue
		}
		_, _ = hash.Write(data)
	}
	return hash.Sum(nil)
}

func suffixAfter(batch *agentv1.ObservationBatch, persisted uint64) *agentv1.ObservationBatch {
	if batch.GetFirstSequence() > persisted {
		return proto.Clone(batch).(*agentv1.ObservationBatch)
	}
	offset := persisted + 1 - batch.GetFirstSequence()
	result := proto.Clone(batch).(*agentv1.ObservationBatch)
	result.FirstSequence = persisted + 1
	result.Observations = result.Observations[offset:]
	result.PayloadChecksum = batchChecksum(result.Observations)
	return result
}

func supportsProtocol(versions []*agentv1.ProtocolVersion) bool {
	for _, version := range versions {
		if version.GetMajor() == protocolMajor {
			return true
		}
	}
	return false
}

func serverHello(sessionID string, persisted uint64, config Config) *agentv1.IngestionToAgent {
	return &agentv1.IngestionToAgent{Frame: &agentv1.IngestionToAgent_Hello{Hello: &agentv1.ServerHello{
		SessionId:               sessionID,
		SelectedProtocolVersion: &agentv1.ProtocolVersion{Major: protocolMajor, Minor: 0},
		AcceptedThroughSequence: persisted,
		MaxBatchRecords:         config.MaxBatchRecords,
		MaxBatchBytes:           config.MaxBatchBytes,
		HeartbeatInterval:       durationpb.New(config.HeartbeatInterval),
		ServerTime:              timestamppb.Now(),
	}}}
}

func acknowledgement(batchID string, received, persisted uint64, retry []*agentv1.SequenceRange, rejections []*agentv1.RecordRejection) *agentv1.IngestionToAgent {
	return &agentv1.IngestionToAgent{Frame: &agentv1.IngestionToAgent_Acknowledgement{Acknowledgement: &agentv1.BatchAcknowledgement{
		BatchId:                  batchID,
		ReceivedThroughSequence:  received,
		PersistedThroughSequence: persisted,
		RetryRanges:              retry,
		Rejections:               rejections,
		AcknowledgedAt:           timestamppb.Now(),
	}}}
}

func flowControl(delay time.Duration, reason string) *agentv1.IngestionToAgent {
	return &agentv1.IngestionToAgent{Frame: &agentv1.IngestionToAgent_FlowControl{FlowControl: &agentv1.FlowControl{
		RetryAfter:         durationpb.New(delay),
		MaxInFlightBatches: 1,
		Reason:             reason,
	}}}
}

func rejectionCode(err error) agentv1.RejectionCode {
	if err != nil && bytes.Contains([]byte(err.Error()), []byte("maximum")) {
		return agentv1.RejectionCode_REJECTION_CODE_TOO_LARGE
	}
	return agentv1.RejectionCode_REJECTION_CODE_INVALID_RECORD
}

func rejectionsForBatch(batch *agentv1.ObservationBatch, code agentv1.RejectionCode, detail string) []*agentv1.RecordRejection {
	if len(batch.GetObservations()) == 0 {
		return []*agentv1.RecordRejection{{
			Sequence:  batch.GetFirstSequence(),
			Code:      code,
			Detail:    detail,
			Retryable: false,
		}}
	}
	rejections := make([]*agentv1.RecordRejection, 0, len(batch.GetObservations()))
	for _, observation := range batch.GetObservations() {
		rejections = append(rejections, &agentv1.RecordRejection{
			Sequence:  observation.GetSequence(),
			Code:      code,
			Detail:    detail,
			Retryable: false,
		})
	}
	return rejections
}

func receiveError(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
