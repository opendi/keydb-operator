# KeyDB Operator

[![Build Status](https://github.com/opendi/keydb-operator/actions/workflows/docker-build.yml/badge.svg)](https://github.com/opendi/keydb-operator/actions/workflows/docker-build.yml)
[![Security Scan](https://github.com/opendi/keydb-operator/actions/workflows/docker-build.yml/badge.svg?event=push)](https://github.com/opendi/keydb-operator/security/code-scanning)
[![Docker Image](https://img.shields.io/docker/v/opendi/keydb-operator?label=docker&sort=semver)](https://hub.docker.com/r/opendi/keydb-operator)
[![Release](https://img.shields.io/github/v/release/opendi/keydb-operator)](https://github.com/opendi/keydb-operator/releases)
[![GitHub Issues](https://img.shields.io/github/issues/opendi/keydb-operator)](https://github.com/opendi/keydb-operator/issues)
[![Kubernetes](https://img.shields.io/badge/kubernetes-1.19%2B-blue)](https://kubernetes.io/)

A modern Kubernetes operator for [KeyDB](https://keydb.dev/), the high-performance Redis alternative, built with the [Operator SDK](https://sdk.operatorframework.io/) in Go.

## Features

### 🚀 **Core Functionality**
- **Multi-Master Mode**: Deploy KeyDB in multi-master configuration with active replication
- **Cluster Mode**: Deploy KeyDB cluster with automatic sharding and high availability
- **Declarative Configuration**: Kubernetes-native resource management
- **Automatic Scaling**: Support for horizontal scaling operations

### 🔒 **Security & Reliability**
- **TLS Encryption**: Full TLS support for client and inter-node communication
- **Authentication**: Password-based authentication with secret management
- **Pod Disruption Budgets**: Ensure cluster availability during maintenance
- **Configuration Validation**: Comprehensive validation to prevent misconfigurations

### 📊 **Monitoring & Observability**
- **Prometheus Metrics**: Built-in Redis exporter for comprehensive monitoring
- **Health Monitoring**: Advanced health checks with replication lag monitoring
- **Status Reporting**: Detailed cluster status and individual node health
- **ServiceMonitor**: Automatic Prometheus ServiceMonitor creation

### 🔄 **Operations & Maintenance**
- **Rolling Upgrades**: Safe, automated rolling upgrades with validation
- **Split-brain Recovery**: Automatic detection and recovery from network partitions
- **Persistent Storage**: Configurable persistent volumes with storage classes
- **Custom Configuration**: Support for custom KeyDB configuration parameters

## Supported KeyDB Modes

### Multi-Master Mode
- Multiple KeyDB instances that can all accept reads and writes
- Active replication between all masters
- Automatic conflict resolution (last-write-wins)
- Ideal for high-write workloads and geographic distribution

### Cluster Mode
- Automatic data sharding across multiple nodes using hash slots
- Master-replica topology for high availability
- Automatic failover and cluster healing
- Supports up to 1000 nodes with 16384 hash slots

## Quick Start

### Prerequisites
- Kubernetes 1.19+
- kubectl configured to access your cluster

### Installation

1. Install the CRDs:
```bash
kubectl apply -f https://raw.githubusercontent.com/opendi/keydb-operator/main/config/crd/bases/keydb.io_keydbclusters.yaml
```

2. Install the operator:
```bash
kubectl apply -f https://github.com/opendi/keydb-operator/releases/latest/download/keydb-operator.yaml
```

Alternatively, you can install a specific version:
```bash
kubectl apply -f https://github.com/opendi/keydb-operator/releases/download/v0.1.0/keydb-operator.yaml
```

3. Create a KeyDB cluster:
```bash
kubectl apply -f - <<EOF
apiVersion: keydb.io/v1alpha1
kind: KeyDBCluster
metadata:
  name: keydb-sample
  namespace: default
spec:
  mode: multi-master
  replicas: 3
  image: eqalpha/keydb:latest
  resources:
    requests:
      memory: "1Gi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "1000m"
  storage:
    size: "10Gi"
EOF
```

4. Check the cluster status:
```bash
kubectl get keydbcluster keydb-sample
kubectl get pods -l app=keydb-sample
```

## Configuration Examples

### Multi-Master Configuration with TLS and Monitoring
```yaml
apiVersion: keydb.io/v1alpha1
kind: KeyDBCluster
metadata:
  name: keydb-multimaster
spec:
  mode: multi-master
  replicas: 3
  image: eqalpha/keydb:latest
  multiMaster:
    activeReplica: true
  config:
    maxMemory: "2gb"
    persistence: true
    requirePass:
      secretRef:
        name: keydb-auth
        key: password
    tls:
      enabled: true
      secretName: keydb-tls-certs
      requireClientCerts: false
    customConfig:
      tcp-keepalive: "300"
      maxclients: "10000"
  resources:
    requests:
      memory: "2Gi"
      cpu: "1000m"
  storage:
    size: "20Gi"
    storageClass: "fast-ssd"
  monitoring:
    enabled: true
    port: 9121
    serviceMonitorLabels:
      prometheus: "kube-prometheus"
  upgrade:
    strategy: "RollingUpdate"
    maxUnavailable: 1
    validationTimeoutSeconds: 300
  podDisruptionBudget:
    maxUnavailable: 1
```

### Cluster Mode Configuration
```yaml
apiVersion: keydb.io/v1alpha1
kind: KeyDBCluster
metadata:
  name: keydb-cluster
spec:
  mode: cluster
  cluster:
    shards: 3
    replicasPerShard: 1
  image: eqalpha/keydb:latest
  config:
    maxMemory: "1gb"
    persistence: true
  resources:
    requests:
      memory: "1Gi"
      cpu: "500m"
  storage:
    size: "10Gi"
```

## API Reference

### KeyDBCluster Spec

| Field | Type | Description |
|-------|------|-------------|
| `mode` | string | Deployment mode: `multi-master` or `cluster` |
| `replicas` | int | Number of KeyDB instances (multi-master mode) |
| `image` | string | KeyDB container image |
| `multiMaster` | object | Multi-master specific configuration |
| `cluster` | object | Cluster mode specific configuration |
| `config` | object | KeyDB configuration options |
| `resources` | object | Resource requirements |
| `storage` | object | Storage configuration |
| `service` | object | Service configuration |
| `monitoring` | object | Monitoring and metrics configuration |
| `upgrade` | object | Rolling upgrade strategy configuration |
| `podDisruptionBudget` | object | Pod disruption budget settings |

### Config Object

| Field | Type | Description |
|-------|------|-------------|
| `maxMemory` | string | Maximum memory usage (e.g., "1gb", "512mb") |
| `persistence` | bool | Enable persistence (default: true) |
| `requirePass` | object | Password authentication configuration |
| `tls` | object | TLS encryption configuration |
| `customConfig` | map | Custom KeyDB configuration parameters |

### TLS Configuration

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable TLS encryption |
| `secretName` | string | Secret containing TLS certificates |
| `requireClientCerts` | bool | Require client certificates |

### Monitoring Configuration

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable Prometheus metrics (default: true) |
| `port` | int | Metrics port (default: 9121) |
| `serviceMonitorLabels` | map | Labels for ServiceMonitor creation |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase: `Pending`, `Running`, `Failed` |
| `replicas` | int | Total number of replicas |
| `readyReplicas` | int | Number of ready replicas |
| `conditions` | []object | Detailed status conditions |
| `nodes` | []object | Individual node status |
| `currentImage` | string | Current image version being used |
| `upgradeStatus` | object | Rolling upgrade progress |
| `health` | object | Cluster health metrics |

### Health Metrics

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Overall health: `Healthy`, `Warning`, `Degraded`, `Critical` |
| `memoryUsagePercent` | float | Average memory usage across nodes |
| `replicationLagSeconds` | int | Maximum replication lag in seconds |
| `connectedClients` | int | Total connected clients |
| `lastCheckTime` | time | Last health check timestamp |

## Development

### Prerequisites
- Go 1.24+
- Docker
- kubectl

### Building from Source
```bash
git clone https://github.com/opendi/keydb-operator.git
cd keydb-operator
make build
```

### Running Locally
```bash
make install run
```

### Running Tests
```bash
make test
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Architecture

### Multi-Master Mode
- All KeyDB instances can accept both reads and writes
- Active replication between all masters using `active-replica yes` and `multi-master yes`
- Automatic conflict resolution with last-write-wins semantics
- Ideal for geographically distributed deployments

### Cluster Mode
- Data automatically sharded across multiple KeyDB nodes using hash slots
- Master-replica topology for high availability
- Automatic failover when masters become unavailable
- Supports up to 1000 nodes with 16384 hash slots

### Operator Features
- **Declarative Management**: Define desired state via Kubernetes custom resources
- **Automatic Configuration**: Generates optimized KeyDB configuration based on deployment mode
- **Advanced Health Monitoring**: Real-time health checks with replication lag and memory monitoring
- **Safe Scaling**: Intelligent scaling with proper data rebalancing and validation
- **Persistent Storage**: Configurable persistent volumes with storage classes
- **Production Security**: Full TLS encryption, authentication, and certificate management
- **Rolling Upgrades**: Zero-downtime upgrades with automatic validation and rollback
- **Split-brain Recovery**: Automatic detection and recovery from network partitions
- **Prometheus Integration**: Built-in metrics collection and ServiceMonitor creation
- **Configuration Validation**: Comprehensive validation to prevent invalid configurations

## Implementation Details

### Custom Resource Definition (CRD)
The operator defines a `KeyDBCluster` custom resource with the following key features:
- Mode selection (multi-master or cluster)
- Resource requirements and limits
- Storage configuration
- Service configuration
- KeyDB-specific settings

### Controller Logic
The controller implements a comprehensive reconciliation loop that:
1. **Configuration Validation**: Validates the KeyDBCluster specification against best practices
2. **Resource Management**: Creates/updates ConfigMaps, StatefulSets, Services, and PodDisruptionBudgets
3. **Rolling Upgrades**: Manages safe rolling upgrades with pod-by-pod validation
4. **Health Monitoring**: Performs continuous health checks and replication monitoring
5. **TLS Management**: Configures TLS encryption and certificate handling
6. **Metrics Collection**: Sets up Prometheus monitoring and ServiceMonitors
7. **Scaling Operations**: Handles intelligent scaling with data safety validation
8. **Status Reporting**: Provides detailed cluster status and health metrics

### Resource Management
- **StatefulSets**: Provides stable network identities and persistent storage with rolling update support
- **Services**: Headless service for StatefulSet discovery, client service for external access, and metrics endpoints
- **ConfigMaps**: Dynamic KeyDB configuration generation with TLS and custom parameter support
- **Secrets**: Secure password and TLS certificate management
- **PodDisruptionBudgets**: Ensures cluster availability during maintenance operations
- **ServiceMonitors**: Automatic Prometheus monitoring configuration

## Advanced Features

### TLS Encryption
```yaml
config:
  tls:
    enabled: true
    secretName: keydb-tls-certs
    requireClientCerts: false
```

### Rolling Upgrades
```yaml
upgrade:
  strategy: "RollingUpdate"
  maxUnavailable: 1
  validationTimeoutSeconds: 300
```

### Prometheus Monitoring
```yaml
monitoring:
  enabled: true
  port: 9121
  serviceMonitorLabels:
    prometheus: "kube-prometheus"
```

### Pod Disruption Budget
```yaml
podDisruptionBudget:
  maxUnavailable: 1
  # OR
  minAvailable: 2
```

### Custom Configuration
```yaml
config:
  customConfig:
    tcp-keepalive: "300"
    maxclients: "10000"
    timeout: "0"
```

## Production Deployment Guide

### 1. TLS Setup
```bash
# Create TLS certificates (example using cert-manager)
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: keydb-tls
spec:
  secretName: keydb-tls-certs
  issuerRef:
    name: ca-issuer
    kind: ClusterIssuer
  dnsNames:
  - "*.keydb-headless.default.svc.cluster.local"
  - "keydb.default.svc.cluster.local"
EOF
```

### 2. Monitoring Setup
```bash
# Ensure Prometheus operator is installed
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml

# Deploy KeyDB cluster with monitoring
kubectl apply -f https://raw.githubusercontent.com/opendi/keydb-operator/main/config/samples/keydb_v1alpha1_keydbcluster_multimaster.yaml
```

### 3. Backup Strategy
```bash
# Example backup using persistent volume snapshots
kubectl patch keydbcluster keydb-sample --type='merge' -p='{"spec":{"config":{"persistence":true}}}'
```

## Troubleshooting

### Common Issues

1. **Pods not starting**: Check resource limits and storage class availability
2. **TLS connection issues**: Verify certificate validity and DNS names
3. **Replication lag**: Monitor memory usage and network connectivity
4. **Upgrade failures**: Check validation timeout and pod readiness

### Debug Commands
```bash
# Check cluster status
kubectl get keydbcluster -o wide

# View detailed status
kubectl describe keydbcluster keydb-sample

# Check pod logs
kubectl logs -l app=keydb -c keydb

# Monitor health metrics
kubectl get keydbcluster keydb-sample -o jsonpath='{.status.health}'
```

## Acknowledgments

- [KeyDB](https://keydb.dev/) - The high-performance Redis alternative
- [Operator SDK](https://sdk.operatorframework.io/) - Framework for building Kubernetes operators
- [krestomatio/keydb-operator](https://github.com/krestomatio/keydb-operator) - Inspiration from the Ansible-based operator
