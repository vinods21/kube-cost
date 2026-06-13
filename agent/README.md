# CK-Kube Agent V1

CK-Kube Agent V1 collects Kubernetes inventory through shared informers and streams typed inventory observations to the ingestion service over the `cost.v1.agent.AgentIngestionService/Connect` gRPC contract.

Collected resources:

- Cluster identity
- Nodes
- Namespaces
- Deployments
- Pods
- Init containers and application containers

Metrics, Kubernetes events, and Karpenter events are not collected in V1.

## Runtime

The agent runs as a leader-elected controller-runtime runnable. Only the elected replica starts informers and the gRPC transport. Readiness becomes healthy after all informer caches synchronize.

Required configuration:

| Environment variable | Purpose |
|---|---|
| `TENANT_ID` | Platform tenant identity |
| `CLUSTER_ID` | Stable platform cluster identity |
| `INGESTION_ENDPOINT` | gRPC target |

TLS is enabled by default. `INSECURE_GRPC=true` is intended only for local development. Optional TLS settings are `TLS_CA_FILE`, `TLS_CERT_FILE`, `TLS_KEY_FILE`, and `TLS_SERVER_NAME`.

## Development

```text
go test ./agent/...
go run ./agent
```

Running outside Kubernetes requires a usable kubeconfig and the required environment variables.
