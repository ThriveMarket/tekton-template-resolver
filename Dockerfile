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

# Final stage
FROM gcr.io/distroless/static:nonroot

# Install git and dependencies
COPY --from=alpine/git:latest /usr/bin/git /usr/bin/git
COPY --from=alpine/git:latest /usr/libexec/git-core /usr/libexec/git-core
COPY --from=alpine/git:latest /usr/share/git-core /usr/share/git-core
COPY --from=alpine/git:latest /lib/ /lib/
COPY --from=alpine/git:latest /usr/lib/ /usr/lib/

# Copy binary from build stage
COPY --from=builder /app/template-resolver /app/template-resolver

# Use nonroot user for security
USER nonroot:nonroot

# Command to run
ENTRYPOINT ["/app/template-resolver"]