
# Build a go app into a minimal docker image with timezone support and SSL cert chains.
FROM golang:latest@sha256:dd08f769578a5f51a22bf6a81109288e23cfe2211f051a5c29bd1c05ad3db52a as builder
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
