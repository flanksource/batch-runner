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
compress: .bin
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
build:  $(LOCALBIN)
	go build -o $(LOCALBIN)/$(NAME) -ldflags "-X \"main.version=$(VERSION_TAG)\"" .

.PHONY: install
install:
	cp $(LOCALBIN)/$(NAME) /usr/local/bin/


LOCALBIN = $(shell pwd)/.bin
ENVTEST ?= $(LOCALBIN)/setup-envtest
ENVTEST_K8S_VERSION = 1.31.0
ENVTEST_VERSION = release-0.19

.PHONY: $(LOCALBIN)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest


.PHONY: test
test: $(ENVTEST)
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use --bin-dir $(LOCALBIN) -p path)"  go run github.com/onsi/ginkgo/v2/ginkgo -v ./...

CONTROLLER_GEN = $(LOCALBIN)/controller-gen

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) object paths="./pkg/apis/..."
	$(CONTROLLER_GEN) crd paths="./pkg/apis/..." output:crd:artifacts:config=chart/crds
	yq -i 'del(.. | .description? | select(.))' chart/crds/batch.flanksource.com_batchtriggers.yaml
	# IMPORTANT: This is to preserve metadata for pod and job spec, without it metadata does not get parsed from batch trigger spec
	yq -i '(.spec.versions[].schema.openAPIV3Schema.properties.spec.properties.pod.properties.metadata.x-kubernetes-preserve-unknown-fields) = true' chart/crds/batch.flanksource.com_batchtriggers.yaml
	yq -i '(.spec.versions[].schema.openAPIV3Schema.properties.spec.properties.job.properties.metadata.x-kubernetes-preserve-unknown-fields) = true' chart/crds/batch.flanksource.com_batchtriggers.yaml
	yq -i '(.spec.versions[].schema.openAPIV3Schema.properties.spec.properties.job.properties.spec.properties.template.properties.metadata.x-kubernetes-preserve-unknown-fields) = true' chart/crds/batch.flanksource.com_batchtriggers.yaml

