FROM golang:1.24 AS builder

# Version information reported by the "/" endpoint of the server.
# They are injected as build arguments by the CI (see .github/workflows/docker-publish.yml).
ARG GIT_SHA=unknown
ARG GIT_TAG=unknown

WORKDIR /go/src/app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
      -ldflags "-s -w \
        -X github.com/ErivanCorreia/torrent-indexer/consts.gitSha=${GIT_SHA} \
        -X github.com/ErivanCorreia/torrent-indexer/consts.gitTag=${GIT_TAG}" \
      -o app .

FROM alpine:latest
LABEL maintainer="erivann47@gmail.com"

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /go/src/app/app .

CMD ["/root/app"]
