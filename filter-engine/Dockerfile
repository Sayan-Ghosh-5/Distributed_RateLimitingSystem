# Stage 1: The Builder (Has g++ and all compiler tools)
FROM alpine:3.18 AS builder
RUN apk add --no-cache g++ 
WORKDIR /app
COPY main.cpp .
# Compile with maximum optimization (-O3) and output a binary named 'guard'
RUN g++ -O3 -static main.cpp -o guard

# Stage 2: The Runner (Tiny production image)
FROM alpine:3.18
WORKDIR /app
# Copy ONLY the compiled binary from Stage 1
COPY --from=builder /app/guard .

# Expose our binary protocol port
EXPOSE 5000

# Run the engine
CMD ["./guard"]