ARG BUILD_FROM=ghcr.io/home-assistant/base:3.22

FROM golang:1.23-alpine AS go-builder

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mgmt ./cmd/mgmt

FROM ${BUILD_FROM}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apk add --no-cache \
	bash \
	bind \
	bind-tools \
	coreutils \
	freeradius \
	haproxy \
	jq \
	netcat-openbsd \
	supervisor

COPY --from=go-builder /out/mgmt /usr/local/bin/mgmt
COPY rootfs /
COPY run.sh /run.sh

RUN cp -an /etc/raddb/. /etc/defaults/radiusd/ \
	&& chmod +x /run.sh \
	&& chmod +x /etc/netservices/*.sh

CMD ["/run.sh"]
