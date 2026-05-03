package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

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
		// Extract the IP address (Simplified for this example)
		ip := strings.Split(r.RemoteAddr, ":")[0]

		// Query the C++ Guard Dog
		status := checkBloomFilter(ip)

		switch status {
		case "RED":
			// Drop the request immediately. Do NOT pass to the database.
			log.Printf("BLOCKED: %s is on the Red List", ip)
			http.Error(w, "429 Too Many Requests - IP Blocked", http.StatusTooManyRequests)
			return

		case "YELLOW":
			// Here you would query Redis to get the exact count.
			// If Redis says they are over the limit, block them.
			log.Printf("WARNING: %s is in the Yellow Zone", ip)
			// Proceed to logic...

		case "GREEN":
			log.Printf("ALLOWED: %s", ip)
		}

		// If we reach here, the IP is safe. Pass the request to the actual endpoint.
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
	// Create a new router
	mux := http.NewServeMux()

	// Register your endpoint, but WRAP it in the RateLimiterMiddleware
	mux.Handle("/api/data", RateLimiterMiddleware(http.HandlerFunc(secureDataHandler)))

	// Start the server
	port := ":8080"
	fmt.Printf("🛡️ Security Gateway running on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, mux))
}
