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
FROM alpine:3.20
RUN apk add --no-cache ca-certificates docker-cli
COPY --from=build /out/skillcloud /skillcloud
EXPOSE 8080
ENTRYPOINT ["/skillcloud"]
