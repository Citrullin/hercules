
FROM scratch

LABEL maintainer="Philipp Brüll <pb@simia.tech>" \
      description="Docker image of the IOTA hercules node"

ADD target/bin/hercules /bin/hercules

VOLUME /data
VOLUME /snapshots

EXPOSE 14265/tcp
EXPOSE 14600/udp

ENV HERCULES_LOG_HELLO="false" \
    HERCULES_LOG_LEVEL="DEBUG" \
    HERCULES_DATABASE_PATH="/data" \
    HERCULES_SNAPSHOTS_PATH="/snapshots"

ENTRYPOINT [ "/bin/hercules" ]
