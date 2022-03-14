FROM golang:1.13-alpine AS builder

WORKDIR /go/src/github.com/keikoproj/governor
COPY . .
RUN apk update && apk add --no-cache build-base make git ca-certificates && update-ca-certificates
ADD https://storage.googleapis.com/kubernetes-release/release/v1.12.3/bin/linux/amd64/kubectl /usr/local/bin/kubectl
RUN chmod 777 /usr/local/bin/kubectl
RUN make build

FROM scratch

COPY --from=builder /go/src/github.com/keikoproj/governor/_output/bin/governor /bin/governor
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/local/bin/kubectl /usr/local/bin/kubectl

CMD ["/bin/governor", "--help"]
