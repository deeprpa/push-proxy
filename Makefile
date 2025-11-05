APP ?= roc
PWD := $(shell pwd)
GO ?= go

# GO='GOOS=windows GOARCH=386 go'
VERSION ?= $(shell git describe --tags | sed 's/\(.*\)-.*/\1/')
GIT_COMMIT = $(shell git rev-parse --short HEAD || echo unsupported)
GO_VERSION = $(shell go version)
APP_VERSION = $(shell git describe --tags --abbrev=0)
BUILD_AT = $(shell date "+%Y-%m-%dT%H:%M:%S")
TIMESTAMP := $(shell date +%s)

IMAGE_NAME ?= ${APP}
IMAGE_TAG = ${VERSION}_${GIT_COMMIT}
IMAGE ?= ${IMAGE_NAME}:${IMAGE_TAG}


ver:
	@echo "Version:   " $(VERSION)
	@echo "Major:     " $(APP_VERSION)
	@echo "Git commit:" $(GIT_COMMIT)
	@echo "Go version:" $(GO_VERSION)
	@echo "OS env:    " $(shell go env GOOS)-$(shell go env GOARCH)
	@echo "Build time:" $(BUILD_AT)


image-tag:
	@echo ${IMAGE_TAG}

run: local
	./bundles/$(APP) -c ${CONFIG_PATH}

local:
	$(GO) build -v -o bundles/$(APP) .

linux:
	GOOS=linux GOARCH=amd64 $(GO) build -v -o bundles/$(APP)-linux .

test:
	$(GO) test -v ./...

test-indocker:
	docker run --rm -i \
	-v ${PWD}:/go/src/${PKGDIR} \
	-w /go/src/${PKGDIR} \
	registry.cn-beijing.aliyuncs.com/wa/hub:golang_1.21 make test

build-image:
	docker build -t ${IMAGE} .

push-image: build-image push-image-exist

push-image-exist:
	docker push ${IMAGE}
