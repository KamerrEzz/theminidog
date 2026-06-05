FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY app/ .
RUN go mod init demoapp && go build -o /out/demoapp .

FROM alpine:3.21
RUN apk --no-cache add ca-certificates wget
RUN mkdir -p /var/log/demoapp
COPY --from=builder /out/demoapp /usr/local/bin/demoapp
ENTRYPOINT ["demoapp"]
