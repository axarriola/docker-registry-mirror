FROM golang:1.12-alpine AS builder

ARG SKOPEO_VERSION

RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev \
    btrfs-progs-dev \
    lvm2-dev \
    gpgme-dev \
    glib-dev || apk update && apk upgrade

# build skopeo
WORKDIR /go/src/github.com/containers/skopeo
RUN git clone -b ${SKOPEO_VERSION} https://github.com/containers/skopeo.git .
#Newer skopeo releases will use bin/skopeo make target
RUN make binary-local DISABLE_CGO=1

# build registry-mirror app
WORKDIR /opt
COPY ./main.go ./go.mod ./go.sum ./
RUN mkdir bin && \
    GO111MODULE=on GOOS=linux GOARCH=amd64 go build -o bin/registry-mirror main.go


FROM alpine:3.7
run apk add --no-cache ca-certificates
# Newer skopeo releases would have the skopeo bin under containers/bin/skopeo
COPY --from=builder /go/src/github.com/containers/skopeo/skopeo /usr/local/bin/skopeo
COPY --from=builder /go/src/github.com/containers/skopeo/default-policy.json /etc/containers/policy.json
COPY --from=builder /opt/bin/registry-mirror ./registry-mirror

CMD ./registry-mirror
