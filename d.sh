#!/usr/bin/env bash
# debug script that compiles frconfig, and copies the binary to the running amster container, starts it up  and port forwards to it.

# To get a shell
#docker run -it --rm -v "$PWD":/go/src/forgerock.io/frconfig  -w /go/src/forgerock.io/frconfig -e GOOS=linux -e GOARCH=amd64 golang:alpine sh

# Linux build for alpine base
#docker run --rm -v "$PWD":/go/src/forgerock.io/frconfigsrv -w /go/src/forgerock.io/frconfigsrv  -e GOOS=linux -e GOARCH=amd64 golang:alpine go build -v

# Linux build for ubuntu
docker run --rm -v "$PWD":/go/src/forgerock.io/frconfigsrv -w /go/src/forgerock.io/frconfigsrv  -e GOOS=linux -e GOARCH=amd64 golang go build -v


pod=`kubectl get pod -l component=amster -o jsonpath='{.items[*].metadata.name}'`

kubectl cp frconfigsrv $pod:/tmp/


echo "Pod: $pod"

#trap 'kill %1; kill %2' SIGINT

trap 'kill $(jobs -p)' EXIT


kubectl port-forward $pod 9080:9080 &

kubectl exec $pod -it /tmp/frconfigsrv




# This is hacky. Find a better way to integrate this in a CI
#cp -f frconfig ~/tmp/fr/forgeops/docker/amster
#cp -f frconfig ~/tmp/fr/forgeops/docker/git

