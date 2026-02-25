FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/miniclaw .

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update \
	&& apt-get install --no-install-recommends -y ca-certificates wget \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/miniclaw /usr/local/bin/miniclaw

ENV MINICLAW_CONFIG=/app/config/config.json

ENTRYPOINT ["miniclaw"]
