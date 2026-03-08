# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o rebindx main.go

# Final stage
FROM alpine:latest

WORKDIR /root/

# Copy the binary from the build stage
COPY --from=builder /app/rebindx .

# DNS runs on port 53 (UDP)
EXPOSE 53/udp

# Web Dashboard runs on port 8080 (TCP)
EXPOSE 8080/tcp

# Run the application
# Use ENTRYPOINT so arguments can be passed easily
ENTRYPOINT ["./rebindx"]
