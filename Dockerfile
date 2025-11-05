# FROM golang:1.20 AS building
FROM registry.cn-beijing.aliyuncs.com/wa/dev:golang_1.23 AS building

ENV APP=account
ENV CGO_ENABLED=0
# ENV GOPROXY=https://goproxy.cn,direct
# ENV GOCACHE=/go/src/owl/.gocache
# ENV GOMODCACHE=/go/src/owl/.gocache/mod

COPY . /go/src/roc

WORKDIR /go/src/roc
RUN make local

# FROM alpine:3.17
FROM registry.cn-beijing.aliyuncs.com/wa/dev:runtime


COPY --from=building /go/src/roc/bundles/account /usr/local/bin/account

EXPOSE 8080
VOLUME [ "/data" ]

ENTRYPOINT ["/usr/local/bin/account"]