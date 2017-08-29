
appname := frconfigsrv

sources := $(wildcard *.go)

build = GOOS=$(1) GOARCH=$(2) CGO_ENABLED=0 go build -o $(appname)$(3)

.PHONY: all linux clean

all: linux

clean:
	rm -rf build/

linux: build/linux_amd64

darwin: build/darwin_amd64

build/linux_amd64: $(sources)
	$(call build,linux,amd64,)

build/darwin_amd64: $(sources)
	$(call build,darwin,amd64,-darwin)


get-deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure
