# Build a static binary, ship it on distroless.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/fiche-agentic ./cmd/fiche-agentic

FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/fiche-agentic /fiche-agentic

# Persisted state (pastes + generated SSH host key) lives here.
# On Railway, attach a Volume mounted at /data so the host key survives
# redeploys (otherwise returning users get host-key-changed warnings).
VOLUME ["/data"]

# HTTP binds to $PORT (Railway injects it); SSH is a fixed TCP port you
# expose via Railway's TCP Proxy.
ENV SSH_ADDR=:2222
EXPOSE 8080 2222

ENTRYPOINT ["/fiche-agentic", "-o", "/data/pastes", "-hostkey", "/data/ssh_host_ed25519_key"]
