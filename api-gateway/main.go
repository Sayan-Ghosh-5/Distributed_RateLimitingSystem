package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Global Redis Client
var rdb *redis.Client
var ctx = context.Background()

func initRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "redis:6379", // Matches the docker-compose service name!
		DB:   0,
	})

	// Test the connection
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("❌ Could not connect to Redis: %v", err)
	}
	fmt.Println("📦 Connected to Redis State Manager")
}

// --- 1. THE MOCK C++ ENGINE INTERFACE ---
// In a real system, this would make a fast TCP or Unix Socket call
// to your running C++ Bloom Filter service.
func checkBloomFilter(ip string) string {
	// 1. Convert the string to a uint32 using your function
	ipInt := ipToUint32(ip)

	// 2. Open a TCP connection to the C++ server
	conn, err := net.Dial("tcp", "filter-engine:5000")
	if err != nil {
		fmt.Println("Error connecting to C++ server:", err)
		return "ERROR"
	}
	// Always close the connection when the function finishes!
	defer conn.Close()

	// 3. Create the 5-byte payload buffer
	payload := make([]byte, 5)

	// Byte 0 is our command: 0x01 (Check IP)
	payload[0] = 0x01

	// Bytes 1-4 hold the IP address. We use BigEndian to write the uint32 into the slice.
	binary.BigEndian.PutUint32(payload[1:5], ipInt)

	// 4. Send the payload
	// YOUR TURN: Use conn.Write() to send the payload
	fmt.Printf("GO SENDING: IP=%s, Uint32=%d\n", ip, ipInt)
	_, err = conn.Write(payload)
	if err != nil {
		fmt.Println("Error sending payload:", err)
		return "ERROR"
	}

	// 5. Receive the 1-byte response
	response := make([]byte, 1)
	// YOUR TURN: Use conn.Read() to read the response into the slice
	_, err = conn.Read(response)
	if err != nil {
		fmt.Println("GO NETWORK ERROR:", err)
		return "ERROR" // Don't let it default to GREEN!
	}

	fmt.Printf("GO RECEIVED: 0x%02x\n", response[0])

	// 6. Interpret the response (0x00 = Green, 0x01 = Yellow, 0x02 = Red)
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

// --- 2. THE RATE LIMIT MIDDLEWARE ---
func RateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := strings.Split(r.RemoteAddr, ":")[0]

		// 1. Query the C++ Guard Dog (L1 Shield)
		status := checkBloomFilter(ip)

		switch status {
		case "RED":
			log.Printf("BLOCKED BY C++: %s", ip)
			http.Error(w, "429 Too Many Requests - Malicious IP", http.StatusTooManyRequests)
			return

		case "GREEN", "YELLOW":
			// 2. The Redis Exact-Count Logic (L2 State Manager)
			redisKey := "rate_limit:" + ip
			limit := int64(5) // Allow 5 requests per minute

			count, err := rdb.Incr(ctx, redisKey).Result()
			if err != nil {
				// If Redis crashes, "Fail-Open" to keep the API alive
				log.Printf("REDIS ERROR: %v", err)
				next.ServeHTTP(w, r)
				return
			}

			if count == 1 {
				rdb.Expire(ctx, redisKey, time.Minute)
			}

			if count > limit {
				log.Printf("RATE LIMITED: %s (Count: %d)", ip, count)
				http.Error(w, "429 Too Many Requests - Quota Exceeded", http.StatusTooManyRequests)
				return
			}

			log.Printf("ALLOWED: %s (Count: %d)", ip, count)
		}

		next.ServeHTTP(w, r)
	})
}

// --- 3. YOUR ACTUAL API ENDPOINTS ---
func secureDataHandler(w http.ResponseWriter, r *http.Request) {
	// This code ONLY runs if the middleware allows it.
	fmt.Fprintf(w, "Welcome to the Secure Database! Here is your sensitive data.")
}

func ipToUint32(ipStr string) uint32 {
	// Parse the string into a GO ip object
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0 // Invalid IP
	}

	// Force it to be a 4-byte IPv4 address (Go handles IPv6 by default)
	ip = ip.To4()

	// Now we have a byte slice: ip[0], ip[1], ip[2], ip[3]
	var result uint32
	// CORRECT: Upgrades to a 32-bit container, THEN shifts the data 24 spaces.
	result = (uint32(ip[0]) << 24) | (uint32(ip[1]) << 16) | (uint32(ip[2]) << 8) | uint32(ip[3])

	return result
}

func main() {
	// Initialize Redis connection
	initRedis()

	// Create a new router
	mux := http.NewServeMux()

	// Register your endpoint, but WRAP it in the RateLimiterMiddleware
	mux.Handle("/api/data", RateLimiterMiddleware(http.HandlerFunc(secureDataHandler)))

	// Start the server
	port := ":8080"
	fmt.Printf("🛡️ Security Gateway running on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, mux))
}
