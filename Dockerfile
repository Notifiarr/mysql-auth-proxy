
# Build a go app into a minimal docker image with timezone support and SSL cert chains.
FROM golang:latest@sha256:18a1f2d1e1d3c49f27c904e9182375169615c65852ace724987929b910195b2c as builder
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_FLAGS=""

RUN mkdir -p $GOPATH/pkg/mod $GOPATH/bin $GOPATH/src /build
COPY . /build
WORKDIR /build

RUN apt update \
    && apt install -y tzdata \
    && go generate ./... \
    && GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 \
       go build ${BUILD_FLAGS} -o /authproxy .

FROM scratch
COPY --from=builder /authproxy /authproxy
# Make sure we have an ssl cert chain and timezone data.
COPY --from=builder /etc/ssl /etc/ssl
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

ENV TZ=UTC

ENTRYPOINT [ "/authproxy" ]
