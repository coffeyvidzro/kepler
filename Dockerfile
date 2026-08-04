ARG GO_VERSION=1.26.5

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

RUN apk add --no-cache \
    ca-certificates \
    git \
    tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN set -eux; \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/server \
        ./cmd/server; \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/worker \
        ./cmd/worker; \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/backoffice \
        ./cmd/backoffice

FROM alpine:3.22

WORKDIR /dugble

RUN apk add --no-cache \
        ca-certificates \
        tzdata \
    && addgroup -S dugble \
    && adduser -S -D -H -G dugble dugble

COPY --from=build /out/server /dugble/server
COPY --from=build /out/worker /dugble/worker
COPY --from=build /out/backoffice /dugble/backoffice

USER dugble

EXPOSE 8080 8081 8082

CMD ["/dugble/server"]
