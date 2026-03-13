FROM golang:1.25.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags='-w -s' \
    -a -installsuffix cgo \
    -o /app/bin/ingestion \
    ./cmd/ingestion && \
    chmod +x /app/bin/ingestion

FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget tzdata

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

COPY --from=builder /app/bin/ingestion /app/ingestion

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE 8080 9090

CMD ["/app/ingestion"]