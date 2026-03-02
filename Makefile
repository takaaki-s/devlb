BINARY=devlb
BUILD_DIR=bin

.PHONY: build test e2e lint fmt clean install

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/devlb

test:
	go test ./...

e2e: build
	DEVLB=$(BUILD_DIR)/$(BINARY) ./scripts/e2e-test.sh

LINT_VERSION=v1.64.8
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(LINT_VERSION) run

fmt:
	gofmt -l -w .

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp $(BUILD_DIR)/$(BINARY) $(HOME)/go/bin/$(BINARY)
