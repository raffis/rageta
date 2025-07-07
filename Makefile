IMG=ghcr.io/rageta/rageta
IMG_DATAPLANE=ghcr.io/rageta/data-plane


# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

rwildcard=$(foreach d,$(wildcard $(addsuffix *,$(1))),$(call rwildcard,$(d)/,$(2)) $(filter $(subst *,%,$(2)),$(d)))

all: lint test build

tidy:
	go mod tidy -compat=1.20

fmt:
	go fmt ./...

.PHONY: test
test:
	go test -race -coverprofile coverage.out -v ./...

GOLANGCI_LINT = $(GOBIN)/golangci-lint
golangci-lint: ## Download golint locally if necessary.
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.52.2)

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=crds
	
lint: golangci-lint
	golangci-lint run --timeout=3m

vet:
	go vet ./...

code-gen:
	./hack/code-gen.sh

build:
	CGO_ENABLED=0 go build -C cmd/cli/ -o ../../rageta
	docker build . -t ghcr.io/rageta/rageta:latest

.PHONY: docker-build
docker-build: build
	docker build -t ${IMG} .

.PHONY: install
install:
	CGO_ENABLED=0 go install .

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/apis/core/...; ./pkg/apis/package/...; ./internal/processor/..."

CONTROLLER_GEN = $(GOBIN)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.15.0)

# go-install-tool will 'go install' any package $2 and install it to $1
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
env -i bash -c "GOBIN=$(GOBIN) PATH=$(PATH) GOPATH=$(shell go env GOPATH) GOCACHE=$(shell go env GOCACHE) go install $(2)" ;\
rm -rf $$TMP_DIR ;\
}
endef
