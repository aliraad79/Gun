FROM --platform=$BUILDPLATFORM golang:1.24.2 AS builder

WORKDIR /code

ENV GOPATH /go
ENV GOCACHE /go-build
ENV GOPROXY https://goproxy.io,direct

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod/cache \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod/cache \
    --mount=type=cache,target=/go-build \
    go build -o bin/app .

FROM debian:bookworm-slim

WORKDIR /usr/local/bin

COPY --from=builder /code/bin/app ./app
COPY .env .

WORKDIR /usr/local/bin
CMD ["./app"]