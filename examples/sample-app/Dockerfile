FROM golang:1 AS builder

COPY . /src

RUN cd /src/examples/sample-app && \
CGO_ENABLED=0 go build -a -ldflags "-s -w -extldflags '-static'" -o app .

RUN cd /src && \
CGO_ENABLED=0 go build -tags=viper_bind_struct -a -ldflags "-s -w -extldflags '-static'" -o delth .

FROM alpine:latest

COPY --from=builder --chown=root:root --chmod=0755 /src/examples/sample-app/app /usr/bin/app
COPY --from=builder --chown=root:root --chmod=0755 /src/delth /usr/bin/delth

RUN apk add --no-cache -U tini curl

ENTRYPOINT ["/sbin/tini", "--", "/usr/bin/delth", "--"]

CMD ["/usr/bin/app"]
