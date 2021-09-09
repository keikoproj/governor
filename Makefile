
.PHONY: test docker clean all

COMMIT=`git rev-parse HEAD`
BUILD=`date +%FT%T%z`
LDFLAG_LOCATION=github.com/keikoproj/governor/cmd/governor/app

LDFLAGS=-ldflags "-X ${LDFLAG_LOCATION}.buildDate=${BUILD} -X ${LDFLAG_LOCATION}.gitCommit=${COMMIT}"


GIT_TAG=$(shell git rev-parse --short HEAD)

IMAGE ?= governor:latest

all: clean build test

build:
	GO111MODULE=on CGO_ENABLED=0 go build ${LDFLAGS} -o _output/bin/governor github.com/keikoproj/governor/cmd/governor

docker-build:
	docker build -t $(IMAGE) .

# Push the docker image
docker-push:
	docker push ${IMAGE}

clean:
	rm -rf _output

test:
	CGO_ENABLED=0 go test -v ./... -coverprofile ./coverage.txt

vtest:
	go test -v ./... -coverprofile ./coverage.txt --logging-enabled

coverage:
	mkdir -p ./_output
	go test -coverprofile ./coverage.txt -v ./...
	go tool cover -html=./coverage.txt -o _output/cover.html
