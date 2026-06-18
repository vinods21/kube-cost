# CK-Kube Agent V1

CK-Kube Agent V1 collects Kubernetes inventory through shared informers and streams typed observations to the ingestion service over the `cost.v1.agent.AgentIngestionService/Connect` gRPC contract. It also samples baseline CPU and memory metrics every 10 seconds.

Collected resources:

- Cluster identity
- Nodes
- Namespaces
- Deployments
- Pods
- Init containers and application containers

Collected metrics:

- Node CPU usage and memory usage from the Kubelet Summary API
- Container CPU usage and working set memory from Metrics API
- Container RSS memory fallback from the Kubelet Summary API when available
- Container CPU and memory requests and limits from Pod specs

GPU metrics, Kubernetes events, and Karpenter events are not collected in V1.

## Runtime

The agent runs as a leader-elected controller-runtime runnable. Only the elected replica starts informers and the gRPC transport. Readiness becomes healthy after all informer caches synchronize.

Required configuration:

| Environment variable | Purpose |
|---|---|
| `TENANT_ID` | Platform tenant identity |
| `CLUSTER_ID` | Stable platform cluster identity |
| `INGESTION_ENDPOINT` | gRPC target |
| `METRICS_INTERVAL` | Metrics sample interval, defaults to `10s` |

TLS is enabled by default. `INSECURE_GRPC=true` is intended only for local development. Optional TLS settings are `TLS_CA_FILE`, `TLS_CERT_FILE`, `TLS_KEY_FILE`, and `TLS_SERVER_NAME`.

Required Kubernetes permissions are read-only access to Nodes, Namespaces, Pods, Deployments, the `metrics.k8s.io` Pod metrics API, and `nodes/proxy` for `/stats/summary`.

## Development

```text
go test ./agent/...
go run ./agent
```

Running outside Kubernetes requires a usable kubeconfig and the required environment variables.
