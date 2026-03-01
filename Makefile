BINARY=devlb
BUILD_DIR=bin

.PHONY: build test lint fmt clean install

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/devlb

test:
	go test ./...

LINT_VERSION=v1.64.8
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(LINT_VERSION) run

fmt:
	gofmt -l -w .

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp $(BUILD_DIR)/$(BINARY) $(HOME)/go/bin/$(BINARY)
