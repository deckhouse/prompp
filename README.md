<h1 align="center" style="border-bottom: none">
    <a href="https://github.com/deckhouse/prompp/blob/legal" target="_blank"><img alt="Prom++" src="/documentation/images/prompp_logo.png"></a><br>Prometheus
</h1>

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

# Install


# Contributing
Refer to [CONTRIBUTING.md](https://github.com/deckhouse/prompp/blob/main/CONTRIBUTING.md)

# License
Apache License 2.0, see [LICENSE](https://github.com/deckhouse/prompp/blob/main/LICENSE).
