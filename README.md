<p align="center">
    <img src="https://github.com/deckhouse/prompp/blob/legal" target="_blank"><img alt="Prom++" src="/documentation/images/prompp_dark_logo.png#gh-light-mode-only" alt="Prom++"/>
    <img src="https://github.com/deckhouse/prompp/blob/legal" target="_blank"><img alt="Prom++" src="/documentation/images/prompp_white_logo.png#gh-dark-mode-only" alt="Prom++"/>
</p>

# Deckhouse Prom++

Deckhouse Prom++ is an open-source, high-performance fork of Prometheus, designed to significantly reduce memory consumption while maintaining full compatibility with the original project.

## Overview

Deckhouse Prom++ builds upon Prometheus, one of the most widely used monitoring and time-series databases. It retains full compatibility with Prometheus, including:
- Configuration files
- API endpoints
- Storage block format
- All functional capabilities

While staying true to Prometheus' core principles, Deckhouse Prom++ introduces major optimizations by rewriting in C++ the most resource-intensive components: in-memory block construction and Write-Ahead Log (WAL) management.

## Key Benefits

- **Drastically reduced memory usage**  
  Through optimized memory handling, Deckhouse Prom++ reduces memory consumption by **up to 10x**, while maintaining full compatibility with existing Prometheus storage formats.

- **Effortless migration**
  Deckhouse Prom++ is a drop-in replacement for Prometheus, allowing users to switch seamlessly without modifying their configurations, data, or workflows.

# Migrating from Prometheus

**Why is WAL conversion needed?**  
Deckhouse Prom++ uses a different WAL (Write-Ahead Log) format but remains fully compatible with historical blocks.  
Since WAL contains **the last 1.5 blocks of data** (typically around **3 hours**), conversion is required to prevent data loss.

### **Convert WAL data manually**

If migrating manually, use the `prompptool` utility included in the release:

#### Convert Prometheus WAL to Deckhouse Prom++ format
```bash
prompptool walvanilla --working-dir <path to prometheus data dir>
```  

#### Convert Deckhouse Prom++ WAL back to Prometheus format

```bash
prompptool walpp --working-dir <path to prometheus data dir>
```  

# Running Deckhouse Prom++

## **Using precompiled binaries**

1. Download the latest binary from the [Releases page](https://github.com/deckhouse/promppold/releases).
2. Run it as a direct replacement for Prometheus:

   ```bash
   ./prompp --config.file=prometheus.yml --storage.tsdb.path=data/
   ```  

## **Using Docker**

Deckhouse Prom++ is available as a Docker image on [Docker Hub](https://hub.docker.com/r/deckhouse/prompp/).

To quickly run a container:

```bash
docker run --name prompp -d -p 127.0.0.1:9090:9090 deckhouse/prompp
```  

Deckhouse Prom++ will be accessible at [http://localhost:9090/](http://localhost:9090/).

# Installing with Prometheus Operator

If you are using **Prometheus Operator**, WAL conversion should be performed **before switching** to Deckhouse Prom++.  
This can be done **automatically** by adding an **init container** to your `Prometheus` resource.

### **Modify prometheus resource**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: example-prometheus
  namespace: monitoring
spec:
...
  image: deckhouse/prompp:<tag> # Replace Prometheus with Deckhouse Prom++
  initContainers:
    - name: prompptool
      image: deckhouse/prompp:<tag>
      command:
        - /bin/prompptool
        - "--working-dir=/prometheus"
        - "walvanilla"
      volumeMounts:
        - name: prometheus-main-db
          mountPath: /prometheus
          subPath: prometheus-db
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop:
            - ALL
        readOnlyRootFilesystem: true
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
...
```  

Apply the updated resource:

```bash
kubectl apply -f prometheus-migration.yaml
```  

Once the WAL conversion completes, Deckhouse Prom++ will seamlessly replace Prometheus.

### **Rolling back to Prometheus**

If you need to roll back to Prometheus, follow these steps:

1. Modify `initContainer` to **convert WAL back to Prometheus format**:

   ```yaml
   command:
     - /bin/prompptool
     - "--working-dir=/prometheus"
     - "--verbose"
     - "walpp"
   ```  

2. **Restore the original Prometheus image** by replacing `deckhouse/prompp:<tag>` with the official Prometheus image:

   ```yaml
   spec:
     image: quay.io/prometheus/prometheus:<tag>
   ```  

3. **Apply the changes again**:

   ```bash
   kubectl apply -f prometheus-migration.yaml
   ```  

After this step, your setup will be rolling back to Prometheus.

# Getting started

Deckhouse Prom++ is fully compatible with Prometheus.  
Once installed, simply replace Prometheus with Deckhouse Prom++ — no configuration changes are needed.

Example configurations can be found [here](https://github.com/deckhouse/prompp/blob/pp/documentation/examples/prometheus.yml).

# Contributing
Refer to [CONTRIBUTING.md](https://github.com/deckhouse/prompp/blob/main/CONTRIBUTING.md)

# License
Apache License 2.0, see [LICENSE](https://github.com/deckhouse/prompp/blob/main/LICENSE).
