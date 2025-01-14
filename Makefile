.PHONY: build build-linux run test test-race test-coverage lint clean

BINARY_NAME=youtube-podcast

build:
	@echo "Building binary..."
	go build -o $(BINARY_NAME)

build-linux:
	@echo "Building binary..."
	GOOS=linux GOARCH=amd64 go build -o /tmp/$(BINARY_NAME)-linux-amd64

run:
	@echo "Running application..."
	go run .

test:
	@echo "Running tests..."
	go test -v ./...

test-race:
	@echo "Running tests with race condition detector enabled..."
	go test -v -race ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -cover ./...

lint:
	@echo "Running linter..."
	golangci-lint run -v ./...

clean:
	@echo "Cleaning up..."
	go clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-linux-amd64
