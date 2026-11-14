FROM alpine:3.24

RUN apk update && apk upgrade --no-cache
RUN adduser -D -u 10001 app

COPY build/ingest-linux /usr/local/bin/ingest
RUN chmod +x /usr/local/bin/ingest && chown app:app /usr/local/bin/ingest

USER app
ENTRYPOINT ["/usr/local/bin/ingest"]
