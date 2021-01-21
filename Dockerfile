FROM alpine:3 as alpine
RUN apk add -U --no-cache ca-certificates
 
FROM scratch
WORKDIR /
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ADD _dist/linux-amd64/gitlab-resources-webhook_linux_amd64 /gitlab-resources-webhook
USER 1000
ENTRYPOINT ["/gitlab-resources-webhook"]
