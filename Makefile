
.PHONY: test docker clean all

COMMIT=`git rev-parse HEAD`
BUILD=`date +%FT%T%z`
LDFLAG_LOCATION=github.com/orkaproj/governor/cmd/governor/app

LDFLAGS=-ldflags "-X ${LDFLAG_LOCATION}.buildDate=${BUILD} -X ${LDFLAG_LOCATION}.gitCommit=${COMMIT}"


GIT_TAG=$(shell git rev-parse --short HEAD)

ifeq (${DOCKER_PUSH},true)
ifndef IMAGE_NAMESPACE
$(error IMAGE_NAMESPACE must be set to push images (e.g. IMAGE_NAMESPACE=docker.mycompany.com))
endif
endif

ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
endif

ifndef IMAGE_TAG
IMAGE_TAG=${GIT_TAG}
endif


# IMAGE is the image name of governer
IMAGE:=$(IMAGE_PREFIX)governor:$(IMAGE_TAG)
IMAGE_LATEST:=$(IMAGE_PREFIX)governor:latest

all: clean build test

build:
	CGO_ENABLED=0 go build ${LDFLAGS} -o _output/bin/governor github.com/orkaproj/governor/cmd/governor

docker:
	docker build -t $(IMAGE) .
	docker tag $(IMAGE) $(IMAGE_LATEST)
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE) ; fi

clean:
	rm -rf _output

test:
	go test -v ./... -coverprofile _output/cover.out

vtest:
	go test -v ./... -coverprofile _output/cover.out --logging-enabled

coverage:
	go test -coverprofile _output/cover.out -v ./...
	go tool cover -html=_output/cover.out -o _output/cover.html
