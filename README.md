## Cloud Native Reverse Proxy

A dynamic multi-source reverse proxy written in Go and inspired by Traefik. Application backends declare themselves and the proxy automatically discovers relevant services, builds routing rules, and load-balances live HTTP traffic to them without configuration changes.

## Features

- **Automatic service discovery**: backends register themselves at runtime. Routing updates the instant an application replica starts or stops, without the need to restart the proxy or make configuration changes.
- **Self-healing routing**: the registry continuously reconciles against what is actually running, so it stays accurate even after a dropped event, a restart, or a network partition.
- **Multi-replica load balancing**: horizontally-scaled backends are balanced automatically, with requests fanned out across every replica behind a host
- **Resilient under failure**: automatically reconnects with backoff when a connection to Docker or Kubernetes drops, keeps routing existing traffic through outages, and drains in-flight requests on graceful shutdown.

##  Architecture 

<img width="1954" height="725" alt="image" src="https://github.com/user-attachments/assets/3c3b135d-1d39-49ff-8e41-7e2c8226b383" />

