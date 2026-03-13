FROM golang:1.25.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags='-w -s' \
    -a -installsuffix cgo \
    -o /app/bin/worker \
    ./cmd/worker && \
    chmod +x /app/bin/worker

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

COPY --from=builder /app/bin/worker /app/worker

RUN chown -R appuser:appuser /app

USER appuser

CMD ["/app/worker"]