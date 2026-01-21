FROM golang:1.25.5 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/gateway ./cmd/gateway

FROM gcr.io/distroless/base-debian12

COPY --from=build /out/gateway /gateway

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/gateway"]
