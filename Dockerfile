FROM alpine:latest
WORKDIR /
COPY rageta rageta

ENTRYPOINT ["/rageta"]
