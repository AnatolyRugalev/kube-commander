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
# CI runs this target in its own workflow (.github/workflows/envtest.yml), never
# as part of the `make check` gate: a failed control-plane download is not a code
# failure (D190). Locally:
#   go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19
ENVTEST_K8S_VERSION ?= 1.31.x
# setup-envtest is a tool, not a module dependency, so it is wherever `go install`
# put it — which is on PATH only if GOPATH/bin is. Look there too rather than
# failing at a command substitution.
SETUP_ENVTEST ?= $(shell command -v setup-envtest 2>/dev/null || printf '%s/bin/setup-envtest' "$$(go env GOPATH)")
test-envtest:
	@set -e; \
	if [ -n "$$KUBEBUILDER_ASSETS" ]; then \
		echo "using KUBEBUILDER_ASSETS=$$KUBEBUILDER_ASSETS"; \
	elif [ -x "$(SETUP_ENVTEST)" ]; then \
		KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use -p path $(ENVTEST_K8S_VERSION))"; \
		export KUBEBUILDER_ASSETS; \
		echo "using KUBEBUILDER_ASSETS=$$KUBEBUILDER_ASSETS"; \
	else \
		echo "make: setup-envtest not found (looked on PATH and at $(SETUP_ENVTEST))." >&2; \
		echo "  go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19" >&2; \
		echo "or set KUBEBUILDER_ASSETS to a control-plane binary directory." >&2; \
		exit 1; \
	fi; \
	KUBECOM_TEST_ENVTEST=1 go test ./internal/kube/... -count=1

# Record the README screencast (docs/screencast.gif) from the committed tape.
# Not part of `check`: it needs vhs (https://github.com/charmbracelet/vhs), a real
# terminal and a real cluster in the current kubeconfig context — see the header of
# docs/screencast/screencast.tape for what the tour assumes, and D181 for why the recording is
# a human's and not an agent's.
#
# The binary is built here and put first on PATH so the recording is always of this
# checkout, never of a stale `kubecom` someone installed months ago.
BIN_DIR ?= $(CURDIR)/bin
screencast:
	go build -o $(BIN_DIR)/kubecom ./cmd/kubecom
	PATH="$(BIN_DIR):$$PATH" vhs docs/screencast/screencast.tape
