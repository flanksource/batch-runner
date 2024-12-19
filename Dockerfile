FROM golang:1.22.5@sha256:86a3c48a61915a8c62c0e1d7594730399caa3feb73655dfe96c7bc17710e96cf AS builder
WORKDIR /app

ARG VERSION

COPY go.mod /app/go.mod
COPY go.sum /app/go.sum
RUN go mod download

COPY ./ ./

RUN make build

FROM debian:bookworm

WORKDIR /app

COPY --from=builder /app/batch-runner /app/batch-runner

ENTRYPOINT ["/app/batch-runner"]
