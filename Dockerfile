# Use the official Go image (1.25 is available as of late 2025)
FROM golang:1.25 AS builder

# These args are automatically populated by buildx
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_REF

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Use the platform-specific args for the build
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 \
  go build -ldflags "-X 'github.com/manisharma/universal-log-streamer/main.Build=${BUILD_REF}'" \
  -o uls *.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/uls /app/uls
ENTRYPOINT ["./uls"]
