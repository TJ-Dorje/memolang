# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN GOTOOLCHAIN=auto go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto go build -o memolang .

# Run stage
FROM alpine:3.21
RUN apk --no-cache add ca-certificates
WORKDIR /app

COPY --from=builder /app/memolang .

# Copy templates and static files
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

EXPOSE 8080

CMD ["./memolang"]
