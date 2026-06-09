BINARY := tmux-parator
CMD := ./cmd/tmux-parator
BIN_DIR := bin
BIN := $(BIN_DIR)/$(BINARY)

.PHONY: fmt tidy test build run popup check clean

fmt:
	go fmt ./...

tidy:
	go mod tidy

test:
	go test ./...

build:
	go build -o $(BIN) $(CMD)

run: build
	$(BIN)

popup: build
	$(BIN) --popup

check: fmt tidy test build

clean:
	rm -rf $(BIN_DIR)
