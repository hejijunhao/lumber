.PHONY: build test lint clean download-model

build:
	go build -o bin/lumber ./cmd/lumber

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

download-model:
	@echo "Downloading ONNX model..."
	@mkdir -p models
	@echo "TODO: add model download URL"
