.PHONY: dev test test-unit test-integration lint build proto clean clean-all

BINARY := nix-key
CMD := ./cmd/nix-key
PROTO_DIR := proto
GEN_DIR := gen

dev:
	go run $(CMD)

test: test-unit test-integration

test-unit:
	go test -race -count=1 -short ./...

test-integration:
	go test -race -count=1 -run Integration ./...

lint:
	golangci-lint run ./...

build:
	go build -o $(BINARY) $(CMD)

proto:
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/nixkey/v1/nix_key.proto

clean:
	rm -f $(BINARY)
	rm -rf coverage/
	rm -rf test-logs/

clean-all: clean
	rm -rf $(GEN_DIR)
	rm -rf vendor/
	go clean -cache -testcache
