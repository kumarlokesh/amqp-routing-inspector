# syntax=docker/dockerfile:1.7

FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -ldflags="-s -w -X github.com/kumarlokesh/amqp-routing-inspector/pkg/version.Version=1.0.0" \
  -o /out/amqp-routing-inspector ./cmd/amqp-routing-inspector

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/amqp-routing-inspector /usr/local/bin/amqp-routing-inspector

ENTRYPOINT ["/usr/local/bin/amqp-routing-inspector"]
