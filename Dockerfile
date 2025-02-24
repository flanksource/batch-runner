FROM golang:1.23.4 AS builder
WORKDIR /app

ARG VERSION

COPY go.mod /app/go.mod
COPY go.sum /app/go.sum
RUN go mod download

COPY ./ ./

RUN make build

FROM debian:bookworm

WORKDIR /app

RUN --mount=type=cache,target=/var/lib/apt \
    --mount=type=cache,target=/var/cache/apt \
    apt-get update  && \
    apt-get install --no-install-recommends -y  curl less locales ca-certificates


COPY --from=builder /app/batch-runner /app/batch-runner

ENTRYPOINT ["/app/batch-runner"]
