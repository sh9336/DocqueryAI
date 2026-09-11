FROM golang:1.25.0-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git gcc musl-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server
 
FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates postgresql-client
COPY --from=builder /app/server .
RUN mkdir -p /app/uploads
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1
CMD ["./server"]
