# Universal Log Streamer (ULS)

A high-performance, Kubernetes-aware log streaming and filtering tool that monitors container and system logs in real-time, filters them based on configurable criteria, and streams matching entries to a remote endpoint.

## Features

- **Kubernetes Native**: Automatically detects Kubernetes environment and extracts pod/container metadata
- **Universal Deployment**: Works in both Kubernetes clusters and standalone VM/bare-metal environments
- **Real-time Log Tailing**: Efficiently monitors log files using k8s informers & fsnotify to tail logs
- **Advanced Filtering**: Supports multiple keywords with AND/OR operators
- **HTTP Status Code Detection**: Special handling for 4xx/5xx status codes with contextual filtering
- **Batch Processing**: Configurable batch sizes for efficient network utilization
- **Namespace Filtering**: Include/exclude specific Kubernetes namespaces
- **Pod Label Filtering**: Filter pods based on Kubernetes labels
- **Graceful Shutdown**: Proper signal handling and resource cleanup
- **Built-in Profiler**: HTTP profiler endpoint for performance monitoring

## Architecture

ULS is designed as a DaemonSet in Kubernetes environments, running on each node to monitor `/var/log/pods`. In non-Kubernetes environments, it can monitor any log directory.

### Key Components

- **Streamer**: Core engine that orchestrates log tailing and filtering
- **Metadata Cache**: Efficient caching of Kubernetes pod/container metadata
- **Regex-based Filtering**: Optimized pattern matching for log entries
- **HTTP Client**: Batch-based log transmission to remote endpoints

## Installation

### Kubernetes Deployment

1. Deploy using the provided manifest:
```bash
kubectl apply -f deploy.yaml
```

The deployment creates:
- ServiceAccount with cluster-level read permissions for pods
- ClusterRole and ClusterRoleBinding
- DaemonSet running on all amd64 nodes

2. Customize the deployment by editing the container args in `deploy.yaml`:
```yaml
args:
  - "--keywords"
  - "error"
  - "--keywords"
  - "exception"
  - "--namespacesToInclude"
  - "your-namespace"
  - "--targetURLWithHostAndScheme"
  - "https://example.com/endpoint"
```

### Docker Build

Build the Docker image:
```bash
docker build -t universal-log-streamer:latest .
```

### Local Development

Run locally with Go:
```bash
go run main.go \
  --keywords "error" \
  --keywords "failed" \
  --targetURLWithHostAndScheme "http://example.com/endpoint" \
  --namespacesToInclude "default"
```

## Configuration

### Command-line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--keywords` | Keywords to filter logs (repeatable) | Required |
| `--operator` | Logical operator for multiple keywords: `or` or `and` | `or` |
| `--namespacesToInclude` | Kubernetes namespaces to include (repeatable) | All namespaces |
| `--namespacesToExclude` | Kubernetes namespaces to exclude (repeatable) | None |
| `--podLabelsToInclude` | Pod labels to filter (format: `key=value`) | None |
| `--targetURLWithHostAndScheme` | Destination URL for log streaming | `https://example.com/endpoint` |
| `--batchSize` | Number of log entries per batch | `10` |
| `--configurationId` | Provider configuration ID for authorization | Empty |
| `--ogranisationId` | Organisation ID for authorization | Empty |
| `--subscriptionId` | Subscription ID for authorization | Empty |
| `--encryptionKey` | Encryption key for payload (planned feature) | Empty |
| `--authToken` | Authentication token for HTTP requests | Empty |

### Special Keywords

- **`4xx`**: Matches HTTP 4xx status codes (400-418, 421-426, 428-429, 431, 451)
  - Must appear with "failed" or "error" in the same log line
- **`5xx`**: Matches HTTP 5xx status codes (500-508, 510-511)
  - Must appear with "failed" or "error" in the same log line

### Filtering Logic

#### OR Operator (default)
Any keyword match triggers log capture:
```bash
--keywords "error" --keywords "timeout" --operator "or"
```
Captures logs containing either "error" **OR** "timeout".

#### AND Operator
All keywords must match:
```bash
--keywords "error" --keywords "payment" --operator "and"
```
Only captures logs containing both "error" **AND** "payment".

## Output Format

Logs are streamed as JSON batches:

```json
[
  {
    "namespace": "default",
    "pod": "api-server-abc123",
    "container": "app",
    "image": "myapp:v1.0.0",
    "imageId": "docker-pullable://myapp@sha256:...",
    "logs": "2026-01-11T10:30:00Z ERROR Failed to connect to database",
    "host": "node-1"
  }
]
```

## Performance

- **Memory**: 100Mi request, 200Mi limit (configurable)
- **CPU**: 50m request, 100m limit (configurable)
- **Profiler**: Available at `http://localhost:6060` for pprof analysis

## Development

### Prerequisites

- Go 1.25.1 or later
- Kubernetes cluster (for testing K8s features)
- Docker (for containerization)

### Project Structure

```
.
├── main.go              # Application entry point
├── internal/
│   ├── flag.go         # Custom flag type for slice arguments
│   ├── object.go       # Data structures (Config, entry)
│   ├── operation.go    # Core operations (tailing, filtering, flushing)
│   └── streamer.go     # Streamer orchestration and initialization
├── Dockerfile          # Multi-stage Docker build
├── deploy.yaml         # Kubernetes deployment manifest
└── go.mod              # Go module dependencies
```

### Dependencies

- `github.com/rs/zerolog` - Structured logging
- `github.com/nxadm/tail` - File tailing
- `github.com/fsnotify/fsnotify` - Filesystem watching
- `k8s.io/client-go` - Kubernetes API client
- `k8s.io/apimachinery` - Kubernetes API types

## Monitoring

The profiler runs on port 6060 and provides:
- CPU profiling: `http://localhost:6060/debug/pprof/profile`
- Memory profiling: `http://localhost:6060/debug/pprof/heap`
- Goroutine dump: `http://localhost:6060/debug/pprof/goroutine`

## Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

For issues and questions, please open an issue on GitHub.
