generate:
	protoc -I ./pb --go_out=paths=source_relative:./pb config.proto
