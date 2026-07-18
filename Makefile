# Canonical verification gate for kubecom (D17). Agents and CI run `make check`;
# "green" means exactly this passing.
# (Replaces the legacy Travis/protoc Makefile; pb/ codegen is gone per D3/D14.)
.PHONY: check build test vet lint test-envtest

check: build test vet lint

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

# Opt-in envtest integration tests (D18): stand up a real kube-apiserver + etcd.
# Not part of `check` — needs control-plane binaries fetched via setup-envtest.
#   go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
ENVTEST_K8S_VERSION ?= 1.31.x
test-envtest:
	KUBEBUILDER_ASSETS="$$(setup-envtest use -p path $(ENVTEST_K8S_VERSION))" \
		KUBECOM_TEST_ENVTEST=1 go test ./internal/kube/... -count=1
