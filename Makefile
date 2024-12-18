NAME=batch-runner
OS   = $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH = $(shell uname -m | sed 's/x86_64/amd64/')

ifeq ($(VERSION),)
  VERSION_TAG=$(shell git describe --abbrev=0 --tags --exact-match 2>/dev/null || echo latest)
else
  VERSION_TAG=$(VERSION)
endif

# Image URL to use all building/pushing image targets
IMG ?= docker.io/flanksource/$(NAME):${VERSION_TAG}

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: tidy
tidy:
	go mod tidy
	git add go.mod go.sum



docker:
	docker build . -f Dockerfile -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}


fmt:
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: compress
compress: .bin/upx
	upx -5 ./.bin/$(NAME)_linux_amd64 ./.bin/$(NAME)_linux_arm64 ./.bin/$(NAME).exe

.PHONY: linux
linux:
	GOOS=linux GOARCH=amd64 go build  -o ./.bin/$(NAME)_linux_amd64 -ldflags "-X \"main.version=$(VERSION_TAG)\""  main.go
	GOOS=linux GOARCH=arm64 go build  -o ./.bin/$(NAME)_linux_arm64 -ldflags "-X \"main.version=$(VERSION_TAG)\""  main.go

.PHONY: darwin
darwin:
	GOOS=darwin GOARCH=amd64 go build -o ./.bin/$(NAME)_darwin_amd64 -ldflags "-X \"main.version=$(VERSION_TAG)\""  main.go
	GOOS=darwin GOARCH=arm64 go build -o ./.bin/$(NAME)_darwin_arm64 -ldflags "-X \"main.version=$(VERSION_TAG)\""  main.go

.PHONY: windows
windows:
	GOOS=windows GOARCH=amd64 go build -o ./.bin/$(NAME).exe -ldflags "-X \"main.version=$(VERSION_TAG)\""  main.go

.PHONY: binaries
binaries: linux darwin windows compress

.PHONY: release
release: binaries
	mkdir -p .release
	cp .bin/$(NAME)* .release/

.PHONY: lint
lint:
	golangci-lint run -v ./...

.PHONY: build
build:
	go build -o $(NAME) -ldflags "-X \"main.version=$(VERSION_TAG)\"" .

.PHONY: install
install:
	cp ./.bin/$(NAME) /usr/local/bin/

.bin/upx: .bin
	wget -nv -O upx.tar.xz https://github.com/upx/upx/releases/download/v3.96/upx-3.96-$(ARCH)_$(OS).tar.xz
	tar xf upx.tar.xz
	mv upx-3.96-$(ARCH)_$(OS)/upx .bin
	rm -rf upx-3.96-$(ARCH)_$(OS)
