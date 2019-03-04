# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=cloudwatch-exporter
COVER_PROFILE_FILE=coverprofile.out
CMD_PATH=./cmd/$(BINARY_NAME)

build:
	$(GOBUILD) -o dist/$(BINARY_NAME) $(CMD_PATH)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o dist/$(BINARY_NAME) $(CMD_PATH)
test:
	$(GOTEST) -v ./...
test-cover:
	$(GOTEST) -coverprofile $(COVER_PROFILE_FILE) -v ./...
test-cover-html:
	$(GOCMD) tool cover -html=$(COVER_PROFILE_FILE)
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
run:
	./dist/$(BINARY_NAME) -conf ./config/dev.yml
run-env:
	./dist/$(BINARY_NAME) -conf ./config/dev-env.yml
run-debug:
	./dist/$(BINARY_NAME) -conf ./config/dev.yml -loglevel="debug"
all: test build
build-run: build run
build-run-debug: build run-debug