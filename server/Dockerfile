FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/skillcloud ./cmd/skillcloud

# Runtime stage uses alpine (not distroless) so the dispatcher can shell out
# to the docker CLI when SKILLCLOUD_DOCKER_BINARY is set. The CLI talks to the
# host's docker daemon over the bind-mounted /var/run/docker.sock (see
# docker-compose.yml).
#
# Runs as a non-root user. To talk to the bind-mounted docker socket, the
# server container needs the host's docker GID as a supplementary group.
# docker-compose.yml wires that up via `group_add` with the DOCKER_GID env
# var (defaults to 999, the docker group GID on most Linux distros).
FROM alpine:3.20
RUN apk add --no-cache ca-certificates docker-cli \
    && addgroup -S skillcloud \
    && adduser -S -G skillcloud -u 65532 skillcloud
COPY --from=build /out/skillcloud /skillcloud
USER skillcloud
EXPOSE 8080
ENTRYPOINT ["/skillcloud"]
