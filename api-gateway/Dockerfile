# Stage 1: The Builder
FROM golang:alpine AS builder
WORKDIR /app
# Copy the module files and download dependencies
COPY go.mod go.sum ./
# (If you had a go.sum, you'd copy it here too: COPY go.mod go.sum ./)
RUN go mod download

# Copy the actual Go code
COPY main.go .
# Build a statically linked Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -o api main.go

# Stage 2: The Runner
FROM alpine:3.18
WORKDIR /app
COPY --from=builder /app/api .

EXPOSE 8080

CMD ["./api"]