APP_NAME  := aiac-service
BUILD_DIR := build
CMD_DIR   := ./cmd/api

.PHONY: dev build run tidy

dev:
	go run $(CMD_DIR)

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

run: build
	./$(BUILD_DIR)/$(APP_NAME)

tidy:
	go mod tidy
	go mod verify
