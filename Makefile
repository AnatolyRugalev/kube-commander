# travis runs `make`, but we don't want to generate anything in CI
all:

generate:
	protoc -I ./pb --go_out=paths=source_relative:./pb config.proto
