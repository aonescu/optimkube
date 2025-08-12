# Kubernetes Cost Optimizer

A comprehensive Kubernetes cost optimization tool similar to Cast AI that tracks resource provisioning, analyzes usage patterns, and provides actionable recommendations to reduce cloud costs.

## Features

- **Real-time Resource Monitoring**: Tracks CPU, memory, and storage usage across nodes and pods
- **Cost Analysis**: Provides detailed cost breakdowns per node, pod, and namespace
- **Smart Recommendations**: Generates optimization suggestions based on usage patterns
- **Automated Actions**: Supports automated scaling and resource optimization
- **RESTful API**: Complete API for integration with other tools
- **Dashboard Ready**: JSON responses ready for frontend dashboards

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Kubernetes    │    │  Cost Optimizer │    │   Monitoring    │
│    Cluster      │◄──►│    Service      │◄──►│    Dashboard    │
│                 │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
        │                       │                       │
        ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Metrics Server │    │ Recommendations │    │   Cost Reports  │
│                 │    │    Engine       │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Prerequisites

- Kubernetes cluster (v1.20+)
- Metrics Server installed and running
- Go 1.21+ (for development)
- Docker (for containerization)
- kubectl access to the cluster

## Quick Start

### 1. Clone the Repository

```bash
git clone <repository-url>
cd optimkube
```

### 2. Build and Deploy

```bash
# Build the Docker image
docker build -t cost-optimizer:latest .

# Deploy to Kubernetes
kubectl apply -f k8s-manifests.yaml
```

### 3. Access the API

```bash
# Port forward to access the service
kubectl port-forward -n kube-system service/cost-optimizer 8080:80

# Test the health endpoint
curl http://localhost:8080/health
```

## API Endpoints

### Cost Analysis

- `GET /api/cost-summary` - Overall cluster cost summary
- `GET /api/metrics/nodes` - Node-level metrics and costs
- `GET /api/metrics/pods` - Pod-level metrics and costs

### Recommendations

- `GET /api/recommendations` - Get optimization recommendations
- `POST /api/optimize` - Trigger immediate cost analysis

### Actions

- `GET /api/actions` - List available optimization actions
- `POST /api/actions/{id}/execute` - Execute optimization action

### Health

- `GET /health` - Service health check

## Usage Examples

### Get Cluster Cost Summary

```bash
curl http://localhost:8080/api/cost-summary | jq
```

Response:
```json
{
  "total_monthly_cost": 450.30,
  "compute_cost": 380.50,
  "storage_cost": 69.80,
  "wasted_resources": 95.20,
  "potential_savings": 127.45,
  "node_count": 5,
  "pod_count": 47,
  "namespace_costs": {
    "default": 125.30,
    "kube-system": 89.70,
    "monitoring": 67.20
  },
  "recommendation_count": 12,
  "last_updated": "2024-01-15T10:30:00Z"
}
```

### Get Optimization Recommendations

```bash
curl http://localhost:8080/api/recommendations | jq
```

Response:
```json
[
  {
    "type": "node_optimization",
    "resource": "node-1",
    "description": "Node node-1 is underutilized (CPU: 15.2%, Memory: 22.1%)",
    "impact": "Consider consolidating workloads or downsizing",
    "potential_savings": 89.50,
    "priority": "medium",
    "timestamp": "2024-01-15T10:30:00Z"
  },
  {
    "type": "resource_rightsizing",
    "resource": "default/nginx-deployment",
    "namespace": "default",
    "description": "Container nginx is over-provisioned for CPU (request: 500m, usage: 150m)",
    "impact": "Reduce CPU request to optimize resource allocation",
    "potential_savings": 15.30,
    "priority": "low",
    "timestamp": "2024-01-15T10:30:00Z"
  }
]
```

## Configuration

The application can be configured through environment variables or the ConfigMap:

### Environment Variables

- `PORT`: Service port (default: 8080)
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `KUBECONFIG`: Path to kubeconfig file (for out-of-cluster access)

### ConfigMap Configuration

Modify the `cost-optimizer-config` ConfigMap to adjust:

- Node instance type costs
- Storage costs
- Monitoring intervals
- Utilization thresholds

## Cost Calculation

### Node Costs

Node costs are calculated based on:
- Instance type hourly rates
- Actual usage vs. capacity
- Reserved vs. on-demand pricing (configurable)

### Pod Costs

Pod costs are estimated using:
- Resource requests and limits
- Proportional node cost allocation
- Storage volume costs

### Waste Detection

The system identifies waste through:
- Low CPU/memory utilization (< 20%/30% thresholds)
- Over-provisioned resource requests
- Idle resources during off-hours
- Unused persistent volumes

## Optimization Strategies

### 1. Right-sizing Resources

- Analyze actual vs. requested resources
- Recommend optimal CPU/memory requests
- Identify over-provisioned workloads

### 2. Horizontal Pod Autoscaling

- Suggest HPA implementation
- Optimize replica counts based on load patterns
- Reduce costs during low-traffic periods

### 3. Node Optimization

- Identify underutilized nodes
- Recommend instance type changes
- Suggest workload consolidation

### 4. Storage Optimization

- Find unused persistent volumes
- Recommend storage class optimization
- Identify oversized volumes

## Development

### Local Development

```bash
# Install dependencies
go mod download

# Run locally (requires kubeconfig)
export KUBECONFIG=~/.kube/config
go run main.go
```

### Testing

```bash
# Run tests
go test ./...

# Run with coverage
go test -cover ./...
```

### Building

```bash
# Build binary
go build -o cost-optimizer main.go

# Build Docker image
docker build -t cost-optimizer:latest .
```

## Monitoring and Observability

### Metrics

The service exposes metrics about:
- Cost calculations
- Recommendation generation
- API request patterns
- Resource utilization trends

### Logging

Structured logging provides insights into:
- Optimization recommendations
- Cost calculations
- API access patterns
- Error conditions

### Health Checks

- `/health` endpoint for liveness/readiness probes
- Kubernetes-native health checking
- Dependency health validation

## Security Considerations

### RBAC

The service requires cluster-wide read access and limited write access:
- Read: nodes, pods, deployments, metrics
- Write: deployments (for scaling), HPA resources

### Authentication

For production deployments:
- Enable ingress authentication
- Use network policies
- Implement API key authentication
- Consider service mesh integration

### Data Privacy

- No sensitive data is stored persistently
- Metrics data is anonymized
- Configuration supports data retention policies

## Troubleshooting

### Common Issues

1. **Metrics Server Not Available**
   ```bash
   kubectl get pods -n kube-system | grep metrics-server
   ```

2. **RBAC Permission Errors**
   ```bash
   kubectl auth can-i get nodes --as=system:serviceaccount:kube-system:cost-optimizer
   ```

3. **High Memory Usage**
   - Adjust monitoring interval
   - Implement data retention policies
   - Optimize metric collection

### Debug Mode

Enable debug logging:
```bash
kubectl set env deployment/cost-optimizer -n kube-system LOG_LEVEL=debug
```

## Roadmap

- [ ] Machine learning-based cost prediction
- [ ] Multi-cloud cost comparison
- [ ] Spot instance optimization
- [ ] Custom resource support
- [ ] Integration with cloud billing APIs
- [ ] Cost allocation by team/project
- [ ] Automated policy enforcement

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For issues and questions:
- Create an issue in the repository
- Check the troubleshooting guide
- Review the API documentation

---

**Note**: This tool provides cost estimates and recommendations. Actual cloud costs may vary based on your specific cloud provider pricing, reserved instances, and other factors. Always validate recommendations in a test environment before applying to production workloads.