<p align="center">
    <img alt="Prom++" src="https://github.com/deckhouse/prompp/blob/pp/documentation/images/prompp_dark_logo.svg#gh-light-mode-only" alt="Prom++" width="391" height="133"/>
    <img alt="Prom++" src="https://github.com/deckhouse/prompp/blob/pp/documentation/images/prompp_white_logo.svg#gh-dark-mode-only" alt="Prom++" width="391" height="133"/>
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

# Getting started

Deckhouse Prom++ is fully compatible with Prometheus.  
Once installed, simply replace Prometheus with Deckhouse Prom++ — no configuration changes are needed.

Example configurations can be found [here](https://github.com/deckhouse/prompp/blob/pp/documentation/examples/prometheus.yml).


# **Installing Deckhouse Prom++**

## **Converting WAL before installation**

Deckhouse Prom++ uses a different WAL (Write-Ahead Log) format but remains fully compatible with historical blocks.  
Since WAL contains **the last 1.5 blocks of data** (typically around **3 hours**), if you plan to use Deckhouse Prom++ as a replacement for Prometheus, WAL conversion is required to prevent data loss.

Refer to the [Migration Guide](#migrating-from-prometheus) for detailed conversion steps.


## **Precompiled binaries**

1. Download the latest binary from the [Releases page](https://github.com/deckhouse/prompp/releases).
2. Run it as a direct replacement for Prometheus:

   ```bash
   ./prompp --config.file=prometheus.yml --storage.tsdb.path=data/
   ```  


## **Docker**

Deckhouse Prom++ is available as a Docker image on [Docker Hub](https://hub.docker.com/r/prompp/prompp/).

To quickly run a container:

```bash
docker run --name prompp -d -p 127.0.0.1:9090:9090 prompp/prompp
```  

Once running, Deckhouse Prom++ will be accessible at [http://localhost:9090/](http://localhost:9090/).


## **Prometheus Operator**

1. Create a file `prompp.yaml` with the following configuration (other settings may be required depending on your setup):

   ```yaml
   apiVersion: monitoring.coreos.com/v1
   kind: Prometheus
   metadata:
     name: example-prometheus
     namespace: monitoring
   spec:
     image: prompp/prompp:<tag>  # Replace Prometheus with Deckhouse Prom++
     securityContext:
       fsGroup: 64535
       runAsGroup: 64535
       runAsNonRoot: true
       runAsUser: 64535 
     # Additional parameters may be required based on your installation
   ```  

2. Apply the updated resource:

   ```bash
   kubectl apply -f prompp.yaml
   ```  


# **Migrating from Prometheus**

## **Manual WAL conversion**

If migrating manually, use the `prompptool` utility included in the release:

### **Convert Prometheus WAL to Deckhouse Prom++ format**

```bash
prompptool walvanilla --working-dir <path to prometheus data dir>
```  

### **Convert Deckhouse Prom++ WAL back to Prometheus format**

```bash
prompptool walpp --working-dir <path to prometheus data dir>
```  

## **Automatic WAL conversion with Prometheus Operator**

### **Converting Prometheus WAL to Deckhouse Prom++ format**

1. Create a file `prompp-migration.yaml` with the following configuration (additional parameters may be required based on your installation):

   ```yaml
   apiVersion: monitoring.coreos.com/v1
   kind: Prometheus
   metadata:
     name: example-prometheus
     namespace: monitoring
   spec:
     ...
     image: prompp/prompp:<tag>  # Replace Prometheus with Deckhouse Prom++
     securityContext:
       fsGroup: 64535
       runAsGroup: 64535
       runAsNonRoot: true
       runAsUser: 64535 
     initContainers:
       - name: prompptool
         image: prompp/prompp:<tag>
         command:
           - /bin/prompptool
           - "--working-dir=/prometheus"
           - "walvanilla"
         volumeMounts:
           - name: prometheus-main-db
             mountPath: /prometheus
             subPath: prometheus-db
         resources:
           requests:
             cpu: "100m"
             memory: "128Mi"
     # Additional parameters may be required based on your installation
   ```  

2. Apply the updated resource:

   ```bash
   kubectl apply -f prompp-migration.yaml
   ```  


### **Convert Deckhouse Prom++ WAL back to Prometheus format**

1. Modify the `initContainer` in your `prompp-migration.yaml` file:

   ```yaml
   command:
     - /bin/prompptool
     - "--working-dir=/prometheus"
     - "--verbose"
     - "walpp"
   ```  

2. Apply the changes again:

   ```bash
   kubectl apply -f prompp-migration.yaml
   ```  


# Contributing
Refer to [CONTRIBUTING.md](https://github.com/deckhouse/prompp/blob/pp/CONTRIBUTING.md)

# License
Apache License 2.0, see [LICENSE](https://github.com/deckhouse/prompp/blob/pp/LICENSE).

# Online community

In addition to common GitHub features, here are some other online resources related to Deckhouse Prom++:

* [Telegram chat](https://t.me/prom_plus_plus) to discuss;
* [Deckhouse blog](https://blog.deckhouse.io) to read the latest articles about all Deckhouse products.
