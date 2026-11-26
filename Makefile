.DEFAULT_GOAL := build

APP_NAME=mu
APP_CMD_DIR=cmd/$(APP_NAME)
APP_BINARY=bin/$(APP_NAME)
APP_BINARY_UNIX=bin/$(APP_NAME)_unix_amd64
SS_SERVER_BINARY=bin/ss-server

all: build

.PHONY: test
test: ## test
	go test -v ./...


.PHONY: build
build: ## build
	CGO_ENABLED=0 go build -o $(APP_BINARY) -v cmd/$(APP_NAME)/main.go

.PHONY: build-ss-server
build-ss-server: ## build the Shadowsocks 2022 multi-user server
	CGO_ENABLED=0 go build -o $(SS_SERVER_BINARY) -v ./cmd/ss-server
