FROM golang
ADD . /go/src/github.com/skilld-labs/etcd
ADD cmd/vendor /go/src/github.com/skilld-labs/etcd/vendor
RUN go install github.com/skilld-labs/etcd
EXPOSE 2379 2380
ENTRYPOINT ["etcd"]
