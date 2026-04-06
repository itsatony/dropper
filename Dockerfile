# --- Build stage ---
FROM golang:1.25-alpine AS build

ARG GIT_COMMIT=unknown
ARG GIT_TAG=unknown
ARG BUILD_TIME=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build \
    -ldflags="-s -w \
        -X github.com/itsatony/go-version.GitCommit=${GIT_COMMIT} \
        -X github.com/itsatony/go-version.GitTag=${GIT_TAG} \
        -X github.com/itsatony/go-version.BuildTime=${BUILD_TIME}" \
    -o /dropper ./cmd/dropper/

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=build /dropper /usr/local/bin/dropper

EXPOSE 8080

ENTRYPOINT ["dropper"]
