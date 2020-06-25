#!/bin/sh

set -e

TAG=${TAG:-latest}
IMAGE_NAME=${IMAGE_NAME:-registry-mirror}
SKOPEO_VERSION=${SKOPEO_VERSION:-v1.1.0}

docker build --network host --build-arg SKOPEO_VERSION=${SKOPEO_VERSION} -t ${IMAGE_NAME}:${TAG} .
docker tag ${IMAGE_NAME}:${TAG} ${IMAGE_NAME}:latest

if [ -n "$REGISTRY" ]; then
    docker tag ${REGISTRY}/${IMAGE_NAME}:${TAG}
    docker push ${REGISTRY}/${IMAGE_NAME}:${TAG}
fi
