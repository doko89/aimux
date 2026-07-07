# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/ai-router .

# Runtime stage
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/ai-router /app/ai-router
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -qO- http://localhost:8080/health || exit 1
CMD ["/app/ai-router"]
