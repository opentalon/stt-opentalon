BINARY_NAME ?= stt-opentalon

.PHONY: build test lint

build:
	go build -o $(BINARY_NAME) .
	@echo "Built: $(BINARY_NAME)"

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run
