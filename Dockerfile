# syntax=docker/dockerfile:1.4

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25.3 AS builder

# Set the working directory
WORKDIR /app

# Copy the Go modules and source code
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build the Go application
RUN go build -o transcode-daemon

# Final stage
FROM --platform=$TARGETPLATFORM linuxserver/ffmpeg:8.0-cli-ls44

# Set the working directory
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /app/transcode-daemon .

# Set the entrypoint to the built binary
ENTRYPOINT ["/app/transcode-daemon"]
