FROM alpine:3.24
RUN apk update && apk upgrade --no-cache
RUN adduser -D -u 10001 app
COPY build/web-linux /usr/local/bin/web
RUN chmod +x /usr/local/bin/web && chown app:app /usr/local/bin/web
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/web"]
