## Cloud Native Reverse Proxy

The Cloud Native Reverse Proxy is a dynamic multi-source reverse proxy written in Go and inspired by Traefik. Application backends declare themselves and the proxy automatically discovers relevant services, builds routing rules, and load-balances live HTTP traffic to them without configuration changes.

## Features

- Automatic service discovery with live routing updates as services start/restart/stop, requiring no proxy restart or configuration changes
- Self-healing route registry that continuously reconciles against running backends, staying accurate through network partitions
- Load-balancing over multiple replicas with distribution of incoming requests across all backends associated with the target host
- Fault-tolerant routing with automatic reconnection on backend failures while ensuring uninterrupted delivery of existing traffic through outages
- Source-scoped route ownership where each provider manages only the routes it discovers, reconciling independently without overwriting one another
- Backpressure-aware event ingestion through bounded internal buffers with a defined overflow policy, absorbing large bursts of engine API events without exhausting memory or stalling the proxy

The Cloud Native Reverse Proxy currently supports deployments on Docker and Kubernetes backends, with ECS and etcd on the roadmap in the near future.

##  Architecture 

<img width="1954" height="725" alt="image" src="https://github.com/user-attachments/assets/3c3b135d-1d39-49ff-8e41-7e2c8226b383" />

