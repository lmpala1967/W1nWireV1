# W1nWire — multi stage build of the server and the client.
#
# The image contains both binaries. The server is the default, the client is
# started with "docker run --entrypoint /usr/local/bin/w1nwire-client ...".

FROM golang:1.26-alpine AS build

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/w1nwire-server ./cmd/w1nwire-server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/w1nwire-client ./cmd/w1nwire-client

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/w1nwire-server /usr/local/bin/w1nwire-server
COPY --from=build /out/w1nwire-client /usr/local/bin/w1nwire-client

# 51820/udp  the tunnel
# 9187/tcp   the metrics of the server
EXPOSE 51820/udp
EXPOSE 9187/tcp

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/w1nwire-server"]
CMD ["-config", "/etc/w1nwire/server.json"]
