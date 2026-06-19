package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"github.com/kube-cost/kube-cost/services/ingestion/persistence"
	"github.com/kube-cost/kube-cost/services/ingestion/queue"
	ingestion "github.com/kube-cost/kube-cost/services/ingestion/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	GRPCAddress             string
	HealthAddress           string
	Insecure                bool
	TLSCertFile             string
	TLSKeyFile              string
	ClientCAFile            string
	QueueCapacity           int
	HighWatermarkPercent    int
	MaxBatchRecords         uint32
	MaxBatchBytes           uint64
	BackpressureDelay       time.Duration
	HeartbeatInterval       time.Duration
	ClickHouseAddress       string
	ClickHouseDatabase      string
	ClickHouseUsername      string
	ClickHousePassword      string
	ClickHouseSecure        bool
	PersistenceBatchSize    int
	PersistenceRetryInitial time.Duration
	PersistenceRetryMaximum time.Duration
	RawArchiveDir           string
	SequenceCheckpointDir   string
}

func ConfigFromEnv() Config {
	return Config{
		GRPCAddress:             envString("GRPC_ADDRESS", ":8080"),
		HealthAddress:           envString("HEALTH_ADDRESS", ":8081"),
		Insecure:                envBool("INGESTION_INSECURE", false),
		TLSCertFile:             envString("INGESTION_TLS_CERT_FILE", "/etc/kube-cost/tls/tls.crt"),
		TLSKeyFile:              envString("INGESTION_TLS_KEY_FILE", "/etc/kube-cost/tls/tls.key"),
		ClientCAFile:            envString("INGESTION_CLIENT_CA_FILE", "/etc/kube-cost/tls/ca.crt"),
		QueueCapacity:           envInt("INGESTION_QUEUE_CAPACITY", 1000),
		HighWatermarkPercent:    envInt("INGESTION_QUEUE_HIGH_WATERMARK_PERCENT", 80),
		MaxBatchRecords:         uint32(envInt("INGESTION_MAX_BATCH_RECORDS", 500)),
		MaxBatchBytes:           uint64(envInt("INGESTION_MAX_BATCH_BYTES", 4<<20)),
		BackpressureDelay:       envDuration("INGESTION_BACKPRESSURE_DELAY", time.Second),
		HeartbeatInterval:       envDuration("INGESTION_HEARTBEAT_INTERVAL", 30*time.Second),
		ClickHouseAddress:       envString("CLICKHOUSE_ADDRESS", "clickhouse:9000"),
		ClickHouseDatabase:      envString("CLICKHOUSE_DATABASE", "kube_cost"),
		ClickHouseUsername:      envString("CLICKHOUSE_USERNAME", "kube_cost"),
		ClickHousePassword:      envString("CLICKHOUSE_PASSWORD", "kube_cost"),
		ClickHouseSecure:        envBool("CLICKHOUSE_SECURE", false),
		PersistenceBatchSize:    envInt("INGESTION_PERSISTENCE_BATCH_SIZE", 20),
		PersistenceRetryInitial: envDuration("INGESTION_PERSISTENCE_RETRY_INITIAL", time.Second),
		PersistenceRetryMaximum: envDuration("INGESTION_PERSISTENCE_RETRY_MAXIMUM", 30*time.Second),
		RawArchiveDir:           envString("INGESTION_RAW_ARCHIVE_DIR", ""),
		SequenceCheckpointDir:   envString("INGESTION_SEQUENCE_CHECKPOINT_DIR", ""),
	}
}

func Run(ctx context.Context, config Config) error {
	if err := validateConfig(config); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", config.GRPCAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", config.GRPCAddress, err)
	}
	defer listener.Close()

	options := []grpc.ServerOption{
		// The configured limit applies to ObservationBatch; reserve framing
		// headroom for the enclosing AgentToIngestion message.
		grpc.MaxRecvMsgSize(int(config.MaxBatchBytes) + 64*1024),
	}
	var authenticator ingestion.Authenticator = ingestion.InsecureAuthenticator{}
	if !config.Insecure {
		credentials, err := serverCredentials(config)
		if err != nil {
			return err
		}
		options = append(options, grpc.Creds(credentials))
		authenticator = ingestion.MTLSAuthenticator{}
	}

	batches := queue.New(config.QueueCapacity)
	clickHouse, err := persistence.OpenClickHouse(persistence.ClickHouseConfig{
		Address:  config.ClickHouseAddress,
		Database: config.ClickHouseDatabase,
		Username: config.ClickHouseUsername,
		Password: config.ClickHousePassword,
		Secure:   config.ClickHouseSecure,
	})
	if err != nil {
		return err
	}
	defer clickHouse.Close()
	pingCtx, cancelPing := context.WithTimeout(ctx, 10*time.Second)
	defer cancelPing()
	if err := clickHouse.Ping(pingCtx); err != nil {
		return fmt.Errorf("connect to ClickHouse: %w", err)
	}
	worker := persistence.NewWorker(persistence.WorkerConfig{
		BatchSize:    config.PersistenceBatchSize,
		RetryInitial: config.PersistenceRetryInitial,
		RetryMaximum: config.PersistenceRetryMaximum,
	}, batches, persistence.NewRepository(clickHouse))
	var archiver ingestion.Archiver = ingestion.NoopArchiver{}
	if config.RawArchiveDir != "" {
		fileArchiver, err := ingestion.NewFileArchiver(config.RawArchiveDir)
		if err != nil {
			return err
		}
		archiver = fileArchiver
	}
	var checkpoints ingestion.CheckpointStore = ingestion.MemoryCheckpointStore{}
	if config.SequenceCheckpointDir != "" {
		fileCheckpoints, err := ingestion.NewFileCheckpointStore(config.SequenceCheckpointDir)
		if err != nil {
			return err
		}
		checkpoints = fileCheckpoints
	}

	grpcServer := grpc.NewServer(options...)
	agentv1.RegisterAgentIngestionServiceServer(grpcServer, ingestion.New(ingestion.Config{
		MaxBatchRecords:      config.MaxBatchRecords,
		MaxBatchBytes:        config.MaxBatchBytes,
		HeartbeatInterval:    config.HeartbeatInterval,
		BackpressureDelay:    config.BackpressureDelay,
		HighWatermarkPercent: config.HighWatermarkPercent,
	}, batches, authenticator, ingestion.WithArchiver(archiver), ingestion.WithCheckpointStore(checkpoints)))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("ingestion", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	ready := make(chan struct{})
	healthHTTP := &http.Server{
		Addr:              config.HealthAddress,
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           healthHandler(ready),
	}
	close(ready)

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsChannel := make(chan error, 3)
	go func() {
		errorsChannel <- worker.Run(runCtx)
	}()
	go func() {
		slog.Info("ingestion gRPC server listening", "address", config.GRPCAddress, "mtls", !config.Insecure)
		errorsChannel <- grpcServer.Serve(listener)
	}()
	go func() {
		slog.Info("ingestion health server listening", "address", config.HealthAddress)
		err := healthHTTP.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			errorsChannel <- err
		}
	}()

	select {
	case err := <-errorsChannel:
		return err
	case <-runCtx.Done():
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		healthServer.SetServingStatus("ingestion", healthpb.HealthCheckResponse_NOT_SERVING)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = healthHTTP.Shutdown(shutdownCtx)
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-shutdownCtx.Done():
			grpcServer.Stop()
		}
		return nil
	}
}

func serverCredentials(config Config) (credentials.TransportCredentials, error) {
	certificate, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load ingestion server certificate: %w", err)
	}
	caData, err := os.ReadFile(config.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("read ingestion client CA: %w", err)
	}
	clientRoots := x509.NewCertPool()
	if !clientRoots.AppendCertsFromPEM(caData) {
		return nil, errors.New("ingestion client CA file contains no certificates")
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientRoots,
	}), nil
}

func validateConfig(config Config) error {
	if config.GRPCAddress == "" || config.HealthAddress == "" {
		return errors.New("gRPC and health addresses are required")
	}
	if config.QueueCapacity < 1 {
		return errors.New("queue capacity must be positive")
	}
	if config.MaxBatchRecords < 1 || config.MaxBatchBytes < 1 {
		return errors.New("batch limits must be positive")
	}
	if config.ClickHouseAddress == "" || config.ClickHouseDatabase == "" || config.ClickHouseUsername == "" {
		return errors.New("ClickHouse address, database, and username are required")
	}
	if config.PersistenceBatchSize < 1 {
		return errors.New("persistence batch size must be positive")
	}
	if !config.Insecure && (config.TLSCertFile == "" || config.TLSKeyFile == "" || config.ClientCAFile == "") {
		return errors.New("server certificate, key, and client CA are required when mTLS is enabled")
	}
	return nil
}

func healthHandler(ready <-chan struct{}) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(writer http.ResponseWriter, _ *http.Request) {
		select {
		case <-ready:
			writer.WriteHeader(http.StatusOK)
		default:
			http.Error(writer, "not ready", http.StatusServiceUnavailable)
		}
	})
	return mux
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
