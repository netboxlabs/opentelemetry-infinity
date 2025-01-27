ARG GOARCH
ARG GOARM

FROM golang:1.23-alpine AS builder

WORKDIR /go/src/github.com/netboxlabs/opentelemetry-infinity
COPY . .
RUN apk update && apk add build-base curl dpkg-dev make
RUN if [ ! -f "runner/otelcol-contrib" ] ; then make getotelcol ; fi
RUN make build && mv build/otlpinf /exe

FROM alpine:3

RUN mkdir /opt/otlpinf/

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /exe /

ENTRYPOINT ["/exe"]
