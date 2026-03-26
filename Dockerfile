FROM golang:1.25.6 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build -trimpath -ldflags "-s -w" -o /out/gateway ./cmd/gateway

FROM alpine:3.21

# Create app user and license directories
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser && \
    mkdir -p /tmp/rbitr /data/rbitr /etc/rbitr && \
    chown -R appuser:appuser /tmp/rbitr /data/rbitr /etc/rbitr

COPY --from=build --chown=appuser:appuser /out/gateway /gateway

USER appuser

EXPOSE 8080

ENTRYPOINT ["/gateway"]
