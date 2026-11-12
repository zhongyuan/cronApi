FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/cronApi ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/cronApi /app/cronApi
COPY configs /app/configs
USER app
EXPOSE 8080
ENTRYPOINT ["/app/cronApi", "-c", "/app/configs/config.yaml"]
