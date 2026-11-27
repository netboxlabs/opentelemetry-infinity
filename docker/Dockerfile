ARG GOARCH
ARG GOARM

FROM golang:1.26-alpine AS builder

# The release whose otelcol-contrib to embed. The workflows pass it so every
# platform embeds the same one: resolving "latest" per platform can land on the
# previous release while GitHub catches up with a new one. Unset, it fetches the
# latest release.
ARG OTELCOL_VERSION

WORKDIR /go/src/github.com/netboxlabs/opentelemetry-infinity
COPY . .
RUN apk update && apk add build-base curl dpkg-dev make upx
# make pastes the release into a shell command, so only a plain version tag is accepted.
RUN if [ -n "$OTELCOL_VERSION" ] && ! echo "$OTELCOL_VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' ; then \
      echo "OTELCOL_VERSION must look like v1.2.3, got: $OTELCOL_VERSION" >&2 ; exit 1 ; \
    fi ; \
    if [ ! -f "runner/otelcol-contrib" ] ; then make getotelcol ${OTELCOL_VERSION:+INF_LATEST_VERSION="$OTELCOL_VERSION"} ; fi
RUN make build && mv build/otlpinf /exe
RUN upx /exe

FROM alpine:3

RUN apk update && apk upgrade --no-cache && mkdir /opt/otlpinf/

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /exe /

ENTRYPOINT ["/exe"]
