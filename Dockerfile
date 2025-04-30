# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -o template-resolver ./cmd/template-resolver

# Final stage with Git and SSH
FROM alpine:latest

# Install git and ssh
RUN apk add --no-cache git openssh-client && \
    # Create non-root user for security
    addgroup -S nonroot && \
    adduser -S nonroot -G nonroot

# Copy binary from build stage
COPY --from=builder /app/template-resolver /app/template-resolver

# Use nonroot user for security
USER nonroot:nonroot

# Command to run
ENTRYPOINT ["/app/template-resolver"]