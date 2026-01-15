# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o fileflow ./cmd/server

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates sqlite wget

WORKDIR /app

COPY --from=builder /app/fileflow .
COPY --from=builder /app/web/static ./web/static
COPY --from=builder /app/deployment/scripts ./scripts

RUN chmod +x ./scripts/*.sh

RUN addgroup -S fileflow && adduser -S -G fileflow -H -u 10001 fileflow
RUN mkdir -p /data && chown -R fileflow:fileflow /app /data

USER fileflow

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q --spider http://localhost:8080/healthz || exit 1

ENTRYPOINT ["./fileflow"]
