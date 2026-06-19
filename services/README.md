# Services

Each child directory is an independently buildable process boundary. Most skeleton binaries expose only the standard gRPC health service; `allocation` exposes the V1 HTTP namespace-cost API, `integrations` can expose the Karpenter V1 scoring API, and `recommendations` contains the V1 CPU/memory rightsizing engine.
