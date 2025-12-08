FROM golang:1.25 AS builder
WORKDIR /app

ARG VERSION

COPY go.mod /app/go.mod
COPY go.sum /app/go.sum
RUN go mod download

COPY ./ ./

RUN make build


FROM flanksource/base-image:latest
ARG TARGETARCH


WORKDIR /app
ENV DEBIAN_FRONTEND=noninteractive


COPY --from=builder /app/batch-runner /app/batch-runner

ENTRYPOINT ["/app/batch-runner"]
