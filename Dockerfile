FROM golang:1.14 as builder
ADD . $GOPATH/src/github.com/atlassian/jec
WORKDIR $GOPATH/src/github.com/atlassian/jec/main
RUN export GIT_COMMIT=$(git rev-list -1 HEAD) && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo \
        -ldflags "-X main.JECCommitVersion=$GIT_COMMIT -X main.JECVersion=1.0.1" -o nocgo -o /jec .
FROM python:alpine3.16 as base
RUN pip install requests
RUN addgroup -S jec && \
    adduser -S jec -G jec && \
    apk update && \
    apk add --no-cache git ca-certificates && \
    update-ca-certificates
COPY --from=builder /jec /opt/jec
RUN mkdir -p /var/log/jec && \
    chown -R jec:jec /var/log/jec && \
    chown -R jec:jec /opt/jec && \
    mkdir -p /var/tmp/jec && \
    chown -R jec:jec /var/tmp/jec
USER jec
ENTRYPOINT ["/opt/jec"]
