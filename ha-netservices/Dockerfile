ARG BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.22
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

COPY rootfs /
COPY run.sh /run.sh

RUN cp -an /etc/raddb/. /etc/defaults/radiusd/ \
	&& chmod +x /run.sh \
	&& chmod +x /etc/netservices/*.sh

CMD ["/run.sh"]
