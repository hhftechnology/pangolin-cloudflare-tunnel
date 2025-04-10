FROM golang:1.23 as builder

WORKDIR /build

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o pangolin-cloudflare-tunnel .

# Create final image
FROM scratch

# Copy SSL certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy the binary from the builder stage
COPY --from=builder /build/pangolin-cloudflare-tunnel /pangolin-cloudflare-tunnel

ENTRYPOINT ["/pangolin-cloudflare-tunnel"]