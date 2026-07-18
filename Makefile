# Canonical verification gate for kubecom (D17). Agents and CI run `make check`;
# "green" means exactly this passing.
# (Replaces the legacy Travis/protoc Makefile; pb/ codegen is gone per D3/D14.)
.PHONY: check build test vet lint

check: build test vet lint

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run
