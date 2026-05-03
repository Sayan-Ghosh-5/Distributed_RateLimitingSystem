# 🛡️ Distributed Security Gateway (Polyglot Rate Limiter)

A high-performance, containerized API Gateway and Rate Limiting system built with **Go** and **C++17**. This project demonstrates advanced distributed systems architecture, utilizing a custom binary protocol over raw POSIX sockets and a mathematically rigorous Scalable Counting Bloom Filter to shed malicious traffic with sub-millisecond latency.

## 🚀 System Architecture

This system uses a two-tier microservice architecture to protect backend resources from DDoS attacks and abusive traffic:

1.  **API Gateway (Go):** A highly concurrent HTTP middleware service that intercepts incoming traffic, extracts client IPs, and queries the security engine.
2.  **Filter Engine (C++):** A blazing-fast, in-memory TCP server running a Scalable Counting Bloom Filter. It processes binary payloads and returns probabilistic access decisions.
3.  **Container Orchestration (Docker):** Both services run in isolated Alpine Linux containers, communicating securely over an internal Docker Bridge Network.

### ✨ Key Features & Engineering Decisions

*   **Custom Binary Protocol:** Bypassed heavy HTTP/REST overhead for inter-process communication. The Go client converts string IP addresses into 32-bit integers (`uint32`) and transmits exactly **5 bytes** over a TCP socket (1 byte for the command, 4 bytes for the IP). The C++ server responds with a **1-byte** status code.
*   **Fail-Open Design:** The Go gateway is designed to "Fail-Open." If the C++ security engine crashes or the network drops, the system logs the error but allows traffic through, prioritizing system availability over strict enforcement.
*   **Zero-Dependency C++ Networking:** Utilized raw POSIX sockets (`<arpa/inet.h>`) and explicit bitwise operations (`<<`, `|`) to handle network byte order (Big-Endian) and type promotion safely.
*   **Optimized Containerization:** Leveraged Docker Multi-Stage builds. The C++ engine compiles natively with `-O3` and `-static` flags, resulting in a production image size of less than 5MB.

## 🧠 The Math: Scalable Counting Bloom Filter

Standard Bloom Filters are static and cannot delete entries. This engine implements a **Scalable Counting Bloom Filter**:
*   Uses an array of integers (instead of bits) to allow for the removal of expired IP bans.
*   Dynamically allocates new memory grids when the capacity threshold is reached, preventing the false-positive rate from degrading under heavy attack loads.

## 🛠️ Getting Started

### Prerequisites
*   Docker and Docker Compose installed.

### Installation & Deployment
Clone the repository and spin up the infrastructure using Docker Compose:
```
git clone [https://github.com/yourusername/distributed-security-gateway.git](https://github.com/yourusername/distributed-security-gateway.git)
cd distributed-security-gateway
docker compose up --build
```

Testing the Gateway
Once the containers are running, open a separate terminal to test the traffic routing.

1. The Happy Path (Allowed Traffic)
Simulate a normal user request:
```
curl http://localhost:8080/api/data
Expected Output: Welcome to the Secure Database! Here is your sensitive data.
```

2. The Blocked Path (Malicious Traffic)
The system is pre-configured to block the Docker Bridge Gateway IP (172.18.0.1) to demonstrate the filter's accuracy. If you test from a local container network, the Go middleware will instantly drop the request:
```
curl http://localhost:8080/api/data
Expected HTTP Response: 429 Too Many Requests - IP Blocked
```

📊 System Tracing & Observability
The system includes built-in terminal tracing to observe the binary protocol in action. During a blocked request, the Docker logs will output:

```
api-gateway-1    | GO SENDING: IP=172.18.0.1, Uint32=2886860801
filter-engine-1  | C++ RECEIVED IP: 172.18.0.1
filter-engine-1  | C++ CHECK RESULT: TRUE (Blocked)
api-gateway-1    | GO RECEIVED: 0x02
api-gateway-1    | BLOCKED: 172.18.0.1 is on the Red List
```

👨‍💻 Tech Stack
Language: Go (1.21+), C++17

Networking: POSIX Sockets, net (Go)

Infrastructure: Docker, Docker Compose, Alpine Linux
