#include <iostream>
#include <vector>
#include <string>
#include <cstdint>
#include <cmath>
#include <functional>
#include <chrono>
#include <random>
#include <bitset>
#include <unistd.h> // for read, write, close

#ifdef _WIN32
#include <winsock2.h>
#include <ws2tcpip.h>
#pragma comment(lib, "Ws2_32.lib")
#else
#include <arpa/inet.h>
#include <netinet/in.h>
#include <unistd.h>
#endif

// ---------------------------------------------------------
// TIER 1: The Core Counting Filter (Dynamically Sized)
// ---------------------------------------------------------
class CountingFilter
{
private:
    std::vector<uint8_t> counters; // Our dynamically allocated memory
    size_t M;                      // Array size
    size_t K;                      // Number of hashes
    size_t current_items;          // How many items are currently in this filter

public:
    size_t capacity_limit; // When do we consider this filter "Full"?

    // (You will need your Kirsch-Mitzenmacher hash helper here again)
    std::vector<size_t> get_hash_indices(const std::string &item) const
    {
        std::vector<size_t> indices;
        indices.reserve(K);
        uint64_t master_hash = std::hash<std::string>{}(item);
        uint32_t hash1 = master_hash >> 32;
        uint32_t hash2 = static_cast<uint32_t>(master_hash);

        for (size_t i = 0; i < K; ++i)
        {
            indices.push_back((hash1 + (i * hash2)) % M);
        }
        return indices;
    }

public:
    // Constructor calculates M and K based on requested capacity
    CountingFilter(size_t capacity, double error_rate)
    {
        capacity_limit = capacity;
        current_items = 0;

        // Calculate M and K based on the math formulas
        double m_double = -(capacity * std::log(error_rate)) / std::pow(std::log(2), 2);
        M = static_cast<size_t>(std::ceil(m_double));

        double k_double = (static_cast<double>(M) / capacity) * std::log(2);
        K = static_cast<size_t>(std::ceil(k_double));

        // Dynamically allocate the exact number of 8-bit counters needed, set to 0
        counters.resize(M, 0);
    }

    void add(const std::string &item)
    {
        std::vector<size_t> indices = get_hash_indices(item);
        for (size_t index : indices)
        {
            // Prevent 8-bit integer overflow (max value is 255)
            if (counters[index] < 255)
            {
                counters[index]++;
            }
        }
        current_items++;
    }
    bool check(const std::string &item) const
    {
        std::vector<size_t> indices = get_hash_indices(item);
        for (size_t index : indices)
        {
            if (counters[index] == 0)
                return false;
        }
        return true;
    }

    // NEW: Returns true if successfully deleted, false if it wasn't there
    bool remove(const std::string &item)
    {
        // Crucial Safety Check: Only delete if it's actually there
        if (!check(item))
            return false;

        std::vector<size_t> indices = get_hash_indices(item);
        for (size_t index : indices)
        {
            if (counters[index] > 0)
            {
                counters[index]--;
            }
        }
        current_items--;
        return true;
    }

    bool is_full() const
    {
        return current_items >= capacity_limit;
    }
};

// ---------------------------------------------------------
// TIER 2: The Scalable Manager
// ---------------------------------------------------------
class ScalableCountingFilter
{
private:
    // A dynamic list of Counting Filters
    std::vector<CountingFilter> filters;

    double target_error;
    size_t base_capacity; // Capacity of the first filter
    double growth_factor; // e.g., 2.0 (next filter is twice as big)
    double tighten_ratio; // default 0.9

    // Calculate P_i = P_0 * (r ^ i)
    double calculate_next_error(size_t index)
    {
        double P0 = target_error * (1.0 - tighten_ratio);
        return P0 * std::pow(tighten_ratio, index);
    }

public:
    ScalableCountingFilter(size_t initial_capacity, double error_rate, double growth = 2.0)
    {
        target_error = error_rate;
        base_capacity = initial_capacity;
        growth_factor = growth;

        // Spawn Filter 0 using the Strict Base Error Rate
        filters.emplace_back(base_capacity, calculate_next_error(0));
    }

    void add(const std::string &item)
    {
        // 1. Check if the LAST filter in the vector is full.
        // 2. If full, calculate new capacity, spawn a new filter, and push_back to vector.
        // 3. Add the item to the LAST filter.

        // 1. Is the current (last) filter full?
        if (filters.back().is_full())
        {

            // 2. Calculate exponential scaling for the new filter
            size_t next_index = filters.size();
            size_t next_capacity = filters.back().capacity_limit * growth_factor;
            double next_error = calculate_next_error(next_index);

            // 3. Spawn the new filter and add it to the chain
            filters.emplace_back(next_capacity, next_error);
        }

        // 4. Always add to the newest filter
        filters.back().add(item);
    }

    bool check(const std::string &item) const
    {
        // 1. Loop through 'filters' in REVERSE (newest first).
        // 2. If any filter returns true, return true.
        // 3. Return false.

        // Traverse backwards: Check newest filters first
        for (auto it = filters.rbegin(); it != filters.rend(); ++it)
        {
            if (it->check(item))
            {
                return true;
            }
        }
        return false;
    }

    bool remove(const std::string &item)
    {
        // 1. Loop through 'filters' in REVERSE.
        // 2. Check if the filter contains the item.
        // 3. If it does, call remove() on THAT filter and return true.

        // Traverse backwards: Delete from the newest match to prevent corruption
        for (auto it = filters.rbegin(); it != filters.rend(); ++it)
        {
            if (it->remove(item))
            {
                return true; // Successfully deleted
            }
        }
        return false; // Item was not found in any filter
    }

    void print_diagnostics() const
    {
        std::cout << "\n--- System Diagnostics ---\n";
        std::cout << "Total Filters Spawned: " << filters.size() << "\n";

        size_t total_capacity = 0;
        for (size_t i = 0; i < filters.size(); ++i)
        {
            total_capacity += filters[i].capacity_limit;
            std::cout << "  Filter " << i << " Capacity: " << filters[i].capacity_limit << "\n";
        }
        std::cout << "Total System Capacity: " << total_capacity << " items\n";
        std::cout << "--------------------------\n\n";
    }
};

// Instantiate the global database (10k initial capacity, 1% error rate)
ScalableCountingFilter bloomFilter(10000, 0.01);

void handle_client(int client_socket)
{
    uint8_t buffer[5];

    // 1. Read exactly 5 bytes
    ssize_t bytes_read = read(client_socket, buffer, 5);

    if (bytes_read == 5)
    {
        uint8_t command = buffer[0];

        if (command == 0x01)
        {
            // 2. Safely cast and reconstruct the 32-bit IP Address
            uint32_t ip_address = ((uint32_t)buffer[1] << 24) |
                                  ((uint32_t)buffer[2] << 16) |
                                  ((uint32_t)buffer[3] << 8) |
                                  ((uint32_t)buffer[4]);

            // Convert back to string for the Bloom Filter (Standard IPv4 format)
            std::string ip_str = std::to_string((ip_address >> 24) & 0xFF) + "." +
                                 std::to_string((ip_address >> 16) & 0xFF) + "." +
                                 std::to_string((ip_address >> 8) & 0xFF) + "." +
                                 std::to_string(ip_address & 0xFF);

            std::cout << "C++ RECEIVED IP: " << ip_str << std::endl;

            // 3. Query the Bloom Filter
            bool is_flagged = bloomFilter.check(ip_str);
            std::cout << "C++ CHECK RESULT: " << (is_flagged ? "TRUE (Blocked)" : "FALSE (Allowed)") << std::endl;
            // bool is_flagged = true; // Hardcoded for testing

            // 4. Send the response
            uint8_t response[1];
            response[0] = is_flagged ? 0x02 : 0x00; // RED (0x02) or GREEN (0x00)

            write(client_socket, response, 1);
        }
    }

    // Always close the socket!
    close(client_socket);
}

int main()
{
    // --- LOAD THE THREAT DATA ---
    std::cout << "Loading threat intelligence data...\n";
    //bloomFilter.add("172.18.0.1"); // Block the Docker Gateway
    bloomFilter.add("10.0.0.5");   // Block some other random IP
    // ----------------------------

    // 1. Create a TCP socket
    int server_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (server_fd == 0)
    {
        std::cerr << "Socket creation failed\n";
        return -1;
    }

    // 2. Bind to Port 5000
    struct sockaddr_in address;
    address.sin_family = AF_INET;
    address.sin_addr.s_addr = INADDR_ANY;
    address.sin_port = htons(5000); // htons ensures the port number is Big-Endian!

    if (bind(server_fd, (struct sockaddr *)&address, sizeof(address)) < 0)
    {
        std::cerr << "Bind failed\n";
        return -1;
    }

    // 3. Start Listening
    if (listen(server_fd, 10) < 0)
    {
        std::cerr << "Listen failed" << std::endl;
        return -1;
    }

    std::cout << "🛡️ C++ Bloom Filter Guard listening on port 5000..." << std::endl;

    // 4. The Infinite Event Loop
    while (true)
    {
        int addrlen = sizeof(address);
        // Block and wait for Go to connect
        int client_socket = accept(server_fd, (struct sockaddr *)&address, (socklen_t *)&addrlen);

        if (client_socket >= 0)
        {
            // Process the Go API's request
            handle_client(client_socket);
        }
    }

    return 0;
}