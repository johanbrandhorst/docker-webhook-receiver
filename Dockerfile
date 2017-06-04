# Build stage
FROM golang AS build-env
ADD . /go/src/github.com/johanbrandhorst/docker-webhook-receiver
ENV CGO_ENABLED=0
RUN cd /go/src/github.com/johanbrandhorst/docker-webhook-receiver && go build -o /app

# Production stage
# Callback requires ca-certificates
FROM docker
COPY --from=broady/cacerts /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build-env /app /
EXPOSE 8080
ENTRYPOINT ["/app"]
