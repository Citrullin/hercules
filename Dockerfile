
FROM scratch

LABEL maintainer="pb@simia.tech" \
      description="Docker image of the IOTA hercules node"

ADD target/bin/hercules /bin/hercules
ADD hercules.docker.config.json /etc/hercules.config.json

VOLUME /data
VOLUME /snapshots

EXPOSE 14265/tcp
EXPOSE 14600/udp

ENTRYPOINT [ "/bin/hercules", "-c", "/etc/hercules.config.json" ]
