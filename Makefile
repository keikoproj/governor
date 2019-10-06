
.PHONY: test docker clean all

COMMIT=`git rev-parse HEAD`
BUILD=`date +%FT%T%z`
LDFLAG_LOCATION=github.com/keikoproj/governor/cmd/governor/app

LDFLAGS=-ldflags "-X ${LDFLAG_LOCATION}.buildDate=${BUILD} -X ${LDFLAG_LOCATION}.gitCommit=${COMMIT}"


GIT_TAG=$(shell git rev-parse --short HEAD)

IMAGE ?= governor:latest

all: clean build test

build:
	CGO_ENABLED=0 go build ${LDFLAGS} -mod=vendor -o _output/bin/governor cmd/governor/governor.go

docker-build:
	docker build -t $(IMAGE) .

# Push the docker image
docker-push:
	docker push ${IMAGE}

clean:
	rm -rf _output

test:
	go test -mod=vendor -v ./... -coverprofile ./coverage.txt

vtest:
	go test -mod=vendor -v ./... -coverprofile ./coverage.txt --logging-enabled

coverage:
	go test -mod=vendor -coverprofile ./coverage.txt -v ./...
	go tool cover -html=./coverage.txt -o _output/cover.html
