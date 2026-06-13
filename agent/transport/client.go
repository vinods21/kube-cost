package transport

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ClientConfig struct {
	TenantID          string
	ClusterID         string
	AgentInstanceID   string
	AgentVersion      string
	KubernetesVersion string
	Endpoint          string
	Insecure          bool
	CAFile            string
	CertFile          string
	KeyFile           string
	ServerName        string
	BatchSize         int
	DialOptions       []grpc.DialOption
}

type Client struct {
	config ClientConfig
	buffer *Buffer
}

func NewClient(config ClientConfig, buffer *Buffer) *Client {
	return &Client{config: config, buffer: buffer}
}

func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := c.connect(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("inventory stream disconnected", "error", err, "retry_after", backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	options, err := c.dialOptions()
	if err != nil {
		return err
	}
	connection, err := grpc.NewClient(c.config.Endpoint, options...)
	if err != nil {
		return fmt.Errorf("create ingestion connection: %w", err)
	}
	defer connection.Close()

	stream, err := agentv1.NewAgentIngestionServiceClient(connection).Connect(ctx)
	if err != nil {
		return fmt.Errorf("open ingestion stream: %w", err)
	}
	if err := stream.Send(&agentv1.AgentToIngestion{
		Frame: &agentv1.AgentToIngestion_Hello{Hello: c.hello()},
	}); err != nil {
		return fmt.Errorf("send agent hello: %w", err)
	}
	first, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("receive server hello: %w", err)
	}
	hello := first.GetHello()
	if hello == nil || hello.GetSelectedProtocolVersion().GetMajor() != 1 {
		return fmt.Errorf("ingestion returned an incompatible handshake")
	}
	c.buffer.Acknowledge(hello.AcceptedThroughSequence, nil)

	frames := make(chan *agentv1.IngestionToAgent)
	receiveErrors := make(chan error, 1)
	go func() {
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				receiveErrors <- receiveErr
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		batchContext, cancel := context.WithTimeout(ctx, time.Second)
		observations, batchErr := c.buffer.Batch(batchContext, c.config.BatchSize)
		cancel()
		if batchErr != nil && !errors.Is(batchErr, context.DeadlineExceeded) && !errors.Is(batchErr, context.Canceled) {
			return batchErr
		}
		if len(observations) == 0 {
			select {
			case <-ctx.Done():
				return nil
			case err := <-receiveErrors:
				return err
			case frame := <-frames:
				if err := c.handleFrame(frame); err != nil {
					return err
				}
			case <-heartbeat.C:
				if err := c.sendHeartbeat(stream); err != nil {
					return err
				}
			default:
			}
			continue
		}

		batch := &agentv1.ObservationBatch{
			BatchId:         fmt.Sprintf("%s-%d-%d", c.config.ClusterID, observations[0].Sequence, observations[len(observations)-1].Sequence),
			FirstSequence:   observations[0].Sequence,
			LastSequence:    observations[len(observations)-1].Sequence,
			CreatedAt:       timestamppb.Now(),
			PayloadChecksum: batchChecksum(observations),
			Observations:    observations,
		}
		if err := stream.Send(&agentv1.AgentToIngestion{
			Frame: &agentv1.AgentToIngestion_Batch{Batch: batch},
		}); err != nil {
			return fmt.Errorf("send observation batch: %w", err)
		}
		if err := c.awaitAcknowledgement(ctx, stream, frames, receiveErrors, heartbeat, batch.BatchId); err != nil {
			return err
		}
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

func (c *Client) awaitAcknowledgement(
	ctx context.Context,
	stream agentv1.AgentIngestionService_ConnectClient,
	frames <-chan *agentv1.IngestionToAgent,
	receiveErrors <-chan error,
	heartbeat *time.Ticker,
	batchID string,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-receiveErrors:
			return err
		case <-heartbeat.C:
			if err := c.sendHeartbeat(stream); err != nil {
				return err
			}
		case frame := <-frames:
			acknowledgement := frame.GetAcknowledgement()
			if acknowledgement == nil {
				if err := c.handleFrame(frame); err != nil {
					return err
				}
				continue
			}
			if acknowledgement.BatchId != "" && acknowledgement.BatchId != batchID {
				continue
			}
			var terminal []uint64
			for _, rejection := range acknowledgement.Rejections {
				if !rejection.Retryable {
					terminal = append(terminal, rejection.Sequence)
				}
			}
			c.buffer.Acknowledge(acknowledgement.PersistedThroughSequence, terminal)
			return nil
		}
	}
}

func (c *Client) handleFrame(frame *agentv1.IngestionToAgent) error {
	if streamError := frame.GetError(); streamError != nil {
		return fmt.Errorf("ingestion stream error %s: %s", streamError.Code, streamError.Detail)
	}
	if flow := frame.GetFlowControl(); flow != nil && flow.RetryAfter != nil {
		time.Sleep(flow.RetryAfter.AsDuration())
	}
	return nil
}

func (c *Client) sendHeartbeat(stream agentv1.AgentIngestionService_ConnectClient) error {
	return stream.Send(&agentv1.AgentToIngestion{
		Frame: &agentv1.AgentToIngestion_Heartbeat{Heartbeat: &agentv1.AgentHeartbeat{
			SentAt:                           timestamppb.Now(),
			HighestSequenceCreated:           c.buffer.HighestSequence(),
			HighestSequencePersistedByServer: c.buffer.PersistedThrough(),
		}},
	})
}

func (c *Client) hello() *agentv1.AgentHello {
	return &agentv1.AgentHello{
		TenantId:          c.config.TenantID,
		ClusterId:         c.config.ClusterID,
		AgentInstanceId:   c.config.AgentInstanceID,
		AgentVersion:      c.config.AgentVersion,
		KubernetesVersion: c.config.KubernetesVersion,
		SupportedProtocolVersions: []*agentv1.ProtocolVersion{
			{Major: 1, Minor: 0},
		},
		Capabilities: []agentv1.Capability{
			agentv1.Capability_CAPABILITY_CLUSTER_INVENTORY,
			agentv1.Capability_CAPABILITY_NODE_INVENTORY,
			agentv1.Capability_CAPABILITY_NAMESPACE_INVENTORY,
			agentv1.Capability_CAPABILITY_DEPLOYMENT_INVENTORY,
			agentv1.Capability_CAPABILITY_POD_INVENTORY,
			agentv1.Capability_CAPABILITY_CONTAINER_INVENTORY,
		},
		ResumeAfterSequence: c.buffer.PersistedThrough(),
	}
}

func (c *Client) dialOptions() ([]grpc.DialOption, error) {
	if len(c.config.DialOptions) > 0 {
		return c.config.DialOptions, nil
	}
	if c.config.Insecure {
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: c.config.ServerName,
	}
	if c.config.CAFile != "" {
		caData, err := os.ReadFile(c.config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read ingestion CA: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system certificate pool: %w", err)
		}
		if !roots.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("ingestion CA file contains no certificates")
		}
		tlsConfig.RootCAs = roots
	}
	if c.config.CertFile != "" {
		certificate, err := tls.LoadX509KeyPair(c.config.CertFile, c.config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load agent client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))}, nil
}
