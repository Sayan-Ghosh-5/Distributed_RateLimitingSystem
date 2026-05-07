package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// ==========================================
// 1. GLOBAL STATE & CONFIGURATION
// ==========================================

var rdb *redis.Client
var ctx = context.Background()

func initRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "redis:6379", // Connects to the Docker Compose service
		DB:   0,
	})
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Printf("⚠️ Redis Warning (Failing Open): %v", err)
	} else {
		fmt.Println("📦 Connected to Redis State Manager")
	}
}

// ==========================================
// 2. WEBSOCKET ARCHITECTURE (THE HUB)
// ==========================================

// AlertMessage is the JSON payload we send to the React frontend
type AlertMessage struct {
	IP     string `json:"ip"`
	Status string `json:"status"` // "ALLOWED", "RATE_LIMITED", "BLOCKED"
	Count  int64  `json:"count"`  // Redis count (0 if blocked by C++)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allows any frontend to connect (useful for local dev)
	},
}

var clients = make(map[*websocket.Conn]bool) // Tracks connected admin dashboards
var broadcast = make(chan AlertMessage)      // Channel to push alerts into
var mutex = &sync.Mutex{}                    // Prevents race conditions with the clients map

// Background worker that listens for alerts and sends them to all dashboards
func handleMessages() {
	for {
		msg := <-broadcast // Wait for a message to arrive in the channel

		mutex.Lock()
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				// If writing fails, the browser tab was closed. Clean up.
				client.Close()
				delete(clients, client)
			}
		}
		mutex.Unlock()
	}
}

// Upgrades the /ws endpoint to a WebSocket connection
func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket Upgrade Error: %v", err)
		return
	}

	mutex.Lock()
	clients[ws] = true
	mutex.Unlock()

	log.Println("🟢 Admin Dashboard Connected via WebSocket!")

	// Keep the connection alive until the client disconnects
	for {
		var msg AlertMessage
		if err := ws.ReadJSON(&msg); err != nil {
			mutex.Lock()
			delete(clients, ws)
			mutex.Unlock()
			break
		}
	}
}

// ==========================================
// 3. C++ SECURITY ENGINE BRIDGE (L1)
// ==========================================

func ipToUint32(ip string) uint32 {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return 0
	}
	ipv4 := parsedIP.To4()
	if ipv4 == nil {
		return 0
	}
	return (uint32(ipv4[0]) << 24) | (uint32(ipv4[1]) << 16) | (uint32(ipv4[2]) << 8) | uint32(ipv4[3])
}

func checkBloomFilter(ip string) string {
	ipInt := ipToUint32(ip)

	conn, err := net.Dial("tcp", "filter-engine:5000")
	if err != nil {
		log.Println("⚠️ C++ Engine unreachable (Failing Open):", err)
		return "ERROR"
	}
	defer conn.Close()

	payload := make([]byte, 5)
	payload[0] = 0x01
	binary.BigEndian.PutUint32(payload[1:5], ipInt)

	if _, err = conn.Write(payload); err != nil {
		return "ERROR"
	}

	response := make([]byte, 1)
	if _, err = conn.Read(response); err != nil {
		return "ERROR"
	}

	switch response[0] {
	case 0x00:
		return "GREEN"
	case 0x01:
		return "YELLOW"
	case 0x02:
		return "RED"
	default:
		return "ERROR"
	}
}

// ==========================================
// 4. THE MASTER MIDDLEWARE
// ==========================================

func RateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := strings.Split(r.RemoteAddr, ":")[0]

		// Step 1: L1 C++ Check
		status := checkBloomFilter(ip)

		if status == "RED" {
			log.Printf("BLOCKED BY C++: %s", ip)
			broadcast <- AlertMessage{IP: ip, Status: "BLOCKED", Count: 0} // Tell Frontend
			http.Error(w, "429 Too Many Requests - Malicious IP", http.StatusTooManyRequests)
			return
		}

		// Step 2: L2 Redis Check
		redisKey := "rate_limit:" + ip
		limit := int64(5)

		count, err := rdb.Incr(ctx, redisKey).Result()
		if err != nil {
			log.Printf("⚠️ Redis Error (Failing Open): %v", err)
			next.ServeHTTP(w, r)
			return
		}

		if count == 1 {
			rdb.Expire(ctx, redisKey, time.Minute)
		}

		if count > limit {
			log.Printf("RATE LIMITED: %s (Count: %d)", ip, count)
			broadcast <- AlertMessage{IP: ip, Status: "RATE_LIMITED", Count: count} // Tell Frontend
			http.Error(w, "429 Too Many Requests - Quota Exceeded", http.StatusTooManyRequests)
			return
		}

		// Step 3: Allowed
		log.Printf("ALLOWED: %s (Count: %d)", ip, count)
		broadcast <- AlertMessage{IP: ip, Status: "ALLOWED", Count: count} // Tell Frontend
		next.ServeHTTP(w, r)
	})
}

// ==========================================
// 5. SERVER ENTRY POINT
// ==========================================

func main() {
	initRedis()

	// Start the WebSocket broadcaster in the background
	go handleMessages()

	mux := http.NewServeMux()

	// The WebSocket Route (NO MIDDLEWARE - Admins don't get rate limited!)
	mux.HandleFunc("/ws", handleConnections)

	// The Protected API Route (WRAPPED IN MIDDLEWARE)
	mux.Handle("/api/data", RateLimiterMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Welcome to the Secure Database! Here is your sensitive data."))
	})))

	fmt.Println("🛡️ Security Gateway running on port :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
