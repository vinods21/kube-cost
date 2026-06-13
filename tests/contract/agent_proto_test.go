package contract_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/wellknownimports"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

var agentSources = []string{
	"cost/v1/common/common.proto",
	"cost/v1/agent/inventory.proto",
	"cost/v1/agent/metrics.proto",
	"cost/v1/agent/events.proto",
	"cost/v1/agent/agent.proto",
}

func TestAgentContractCompilesAndIsBidirectional(t *testing.T) {
	files := compileAgentContract(t)
	registry := registerFiles(t, files)

	descriptor, err := registry.FindDescriptorByName("cost.v1.agent.AgentIngestionService")
	if err != nil {
		t.Fatal(err)
	}
	service := descriptor.(protoreflect.ServiceDescriptor)
	connect := service.Methods().ByName("Connect")
	if connect == nil {
		t.Fatal("Connect method is missing")
	}
	if !connect.IsStreamingClient() || !connect.IsStreamingServer() {
		t.Fatal("Connect must be bidirectional streaming")
	}
}

func TestObservationSupportsAllRequiredPayloads(t *testing.T) {
	registry := registerFiles(t, compileAgentContract(t))
	descriptor, err := registry.FindDescriptorByName("cost.v1.agent.Observation")
	if err != nil {
		t.Fatal(err)
	}
	observation := descriptor.(protoreflect.MessageDescriptor)
	payload := observation.Oneofs().ByName("payload")
	if payload == nil {
		t.Fatal("Observation.payload oneof is missing")
	}

	required := []protoreflect.Name{
		"cluster_inventory",
		"node_inventory",
		"namespace_inventory",
		"deployment_inventory",
		"pod_inventory",
		"container_inventory",
		"node_metrics",
		"container_metrics",
		"gpu_metrics",
		"kubernetes_event",
		"karpenter_event",
	}
	for _, name := range required {
		if payload.Fields().ByName(name) == nil {
			t.Errorf("Observation.payload is missing %s", name)
		}
	}
}

func TestMetricsDistinguishMissingFromZero(t *testing.T) {
	registry := registerFiles(t, compileAgentContract(t))
	for _, messageName := range []protoreflect.FullName{
		"cost.v1.agent.NodeMetrics",
		"cost.v1.agent.ContainerMetrics",
		"cost.v1.agent.GpuMetrics",
	} {
		descriptor, err := registry.FindDescriptorByName(messageName)
		if err != nil {
			t.Fatal(err)
		}
		fields := descriptor.(protoreflect.MessageDescriptor).Fields()
		for i := 0; i < fields.Len(); i++ {
			field := fields.Get(i)
			if field.Kind() == protoreflect.Int64Kind ||
				field.Kind() == protoreflect.Uint64Kind ||
				field.Kind() == protoreflect.DoubleKind {
				if !field.HasPresence() {
					t.Errorf("%s.%s must preserve scalar presence", messageName, field.Name())
				}
			}
		}
	}
}

func TestInventorySupportsSnapshotBoundaries(t *testing.T) {
	registry := registerFiles(t, compileAgentContract(t))
	descriptor, err := registry.FindDescriptorByName("cost.v1.agent.Observation")
	if err != nil {
		t.Fatal(err)
	}
	payload := descriptor.(protoreflect.MessageDescriptor).Oneofs().ByName("payload")
	if payload.Fields().ByName("inventory_snapshot_marker") == nil {
		t.Fatal("Observation.payload is missing inventory_snapshot_marker")
	}
}

func TestAgentExamplesAreValid(t *testing.T) {
	registry := registerFiles(t, compileAgentContract(t))
	root := repositoryRoot(t)

	examples := map[string]protoreflect.FullName{
		"01-agent-hello.json":     "cost.v1.agent.AgentToIngestion",
		"02-server-hello.json":    "cost.v1.agent.IngestionToAgent",
		"03-inventory-batch.json": "cost.v1.agent.AgentToIngestion",
		"04-telemetry-batch.json": "cost.v1.agent.AgentToIngestion",
		"05-acknowledgement.json": "cost.v1.agent.IngestionToAgent",
	}

	for name, messageName := range examples {
		t.Run(name, func(t *testing.T) {
			descriptor, err := registry.FindDescriptorByName(messageName)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, "proto", "examples", "agent", name))
			if err != nil {
				t.Fatal(err)
			}
			message := dynamicpb.NewMessage(descriptor.(protoreflect.MessageDescriptor))
			if err := protojson.Unmarshal(data, message); err != nil {
				t.Fatalf("invalid example: %v", err)
			}
			validateFrameInvariants(t, message)
		})
	}
}

func TestUnknownFieldsSurviveRoundTrip(t *testing.T) {
	registry := registerFiles(t, compileAgentContract(t))
	descriptor, err := registry.FindDescriptorByName("cost.v1.agent.AgentHello")
	if err != nil {
		t.Fatal(err)
	}
	message := dynamicpb.NewMessage(descriptor.(protoreflect.MessageDescriptor))
	unknown := protowire.AppendTag(nil, 500, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 42)

	if err := proto.Unmarshal(unknown, message); err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, unknown) {
		t.Fatal("unknown field was not preserved")
	}
}

func TestEnumsUseUnspecifiedZeroValue(t *testing.T) {
	files := compileAgentContract(t)
	for _, file := range files {
		checkEnumZeroValues(t, file)
	}
}

func compileAgentContract(t *testing.T) linker.Files {
	t.Helper()
	root := repositoryRoot(t)
	resolver := &protocompile.SourceResolver{
		ImportPaths: []string{filepath.Join(root, "proto")},
	}
	compiler := protocompile.Compiler{
		Resolver: wellknownimports.WithStandardImports(resolver),
	}
	files, err := compiler.Compile(context.Background(), agentSources...)
	if err != nil {
		t.Fatalf("compile agent contract: %v", err)
	}
	return files
}

func registerFiles(t *testing.T, files linker.Files) *protoregistry.Files {
	t.Helper()
	registry := new(protoregistry.Files)
	for _, file := range files {
		if err := registry.RegisterFile(file); err != nil {
			t.Fatalf("register %s: %v", file.Path(), err)
		}
	}
	return registry
}

func validateFrameInvariants(t *testing.T, message *dynamicpb.Message) {
	t.Helper()
	frame := message.Descriptor().Oneofs().ByName("frame")
	if frame == nil {
		return
	}
	selected := message.WhichOneof(frame)
	if selected == nil {
		t.Fatal("stream frame has no selected payload")
	}

	value := message.Get(selected).Message()
	switch selected.Name() {
	case "batch":
		first := uintField(value, "first_sequence")
		last := uintField(value, "last_sequence")
		observations := value.Get(value.Descriptor().Fields().ByName("observations")).List()
		if observations.Len() == 0 || last < first {
			t.Fatal("batch has an invalid sequence range")
		}
		if uint64(observations.Len()) != last-first+1 {
			t.Fatal("batch observations must cover the complete sequence range")
		}
		for i := 0; i < observations.Len(); i++ {
			if got, want := uintField(observations.Get(i).Message(), "sequence"), first+uint64(i); got != want {
				t.Fatalf("observation sequence %d, want %d", got, want)
			}
		}
	case "acknowledgement":
		received := uintField(value, "received_through_sequence")
		persisted := uintField(value, "persisted_through_sequence")
		if persisted > received {
			t.Fatal("persisted acknowledgement cannot exceed received acknowledgement")
		}
	}
}

func uintField(message protoreflect.Message, name protoreflect.Name) uint64 {
	return message.Get(message.Descriptor().Fields().ByName(name)).Uint()
}

func checkEnumZeroValues(t *testing.T, file protoreflect.FileDescriptor) {
	t.Helper()
	var checkEnums func(protoreflect.EnumDescriptors)
	checkEnums = func(enums protoreflect.EnumDescriptors) {
		for i := 0; i < enums.Len(); i++ {
			enum := enums.Get(i)
			zero := enum.Values().ByNumber(0)
			if zero == nil || !strings.HasSuffix(string(zero.Name()), "_UNSPECIFIED") {
				t.Errorf("%s zero value must end in _UNSPECIFIED", enum.FullName())
			}
		}
	}
	var checkMessages func(protoreflect.MessageDescriptors)
	checkMessages = func(messages protoreflect.MessageDescriptors) {
		for i := 0; i < messages.Len(); i++ {
			message := messages.Get(i)
			checkEnums(message.Enums())
			checkMessages(message.Messages())
		}
	}
	checkEnums(file.Enums())
	checkMessages(file.Messages())
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
