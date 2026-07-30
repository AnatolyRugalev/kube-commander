# Canonical verification gate for kubecom (D17). Agents and CI run `make check`;
# "green" means exactly this passing.
# (Replaces the legacy Travis/protoc Makefile; pb/ codegen is gone per D3/D14.)
.PHONY: check build test vet lint test-envtest keys-doc screencast

check: build test vet lint

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

# Regenerate the committed keybindings reference (docs/keybindings.md) from the
# default keymap (D11). `make check` fails if the doc drifts (TestKeybindingsDoc).
keys-doc:
	go test ./internal/tui/keymap -run TestKeybindingsDoc -update

# Opt-in envtest integration tests (D18): stand up a real kube-apiserver + etcd.
# Not part of `check` — needs control-plane binaries fetched via setup-envtest.
#   go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
ENVTEST_K8S_VERSION ?= 1.31.x
test-envtest:
	KUBEBUILDER_ASSETS="$$(setup-envtest use -p path $(ENVTEST_K8S_VERSION))" \
		KUBECOM_TEST_ENVTEST=1 go test ./internal/kube/... -count=1

# Record the README screencast (docs/screencast.gif) from the committed tape.
# Not part of `check`: it needs vhs (https://github.com/charmbracelet/vhs), a real
# terminal and a real cluster in the current kubeconfig context — see the header of
# docs/screencast.tape for what the tour assumes, and D181 for why the recording is
# a human's and not an agent's.
#
# The binary is built here and put first on PATH so the recording is always of this
# checkout, never of a stale `kubecom` someone installed months ago.
BIN_DIR ?= $(CURDIR)/bin
screencast:
	go build -o $(BIN_DIR)/kubecom ./cmd/kubecom
	PATH="$(BIN_DIR):$$PATH" vhs docs/screencast.tape
