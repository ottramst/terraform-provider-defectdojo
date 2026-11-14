NAME = defectdojo
BINARY = terraform-provider-${NAME}

default: fmt lint install generate

build:
	go build -o $(BINARY) -v .

.PHONY: build-ci
build-ci:
	go build -o $(BINARY)

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

# The supported DefectDojo versions, oldest first. .github/defectdojo-versions.json
# is the only place they are listed.
DEFECTDOJO_VERSIONS := $(shell jq -r '.versions | join(" ")' .github/defectdojo-versions.json)
DD_VERSION ?= $(lastword $(DEFECTDOJO_VERSIONS))

# Starts the acceptance test stack: DefectDojo $(DD_VERSION) on localhost:8080.
up:
	DD_VERSION=$(DD_VERSION) docker compose up -d --wait

# Stops it and deletes its data; needed before switching versions.
down:
	DD_VERSION=$(DD_VERSION) docker compose down -v

# Needs a running DefectDojo (make up). The defaults are the local stack's
# superuser; set the variables to test against another server.
export DEFECTDOJO_URL ?= http://localhost:8080
ifeq ($(DEFECTDOJO_API_TOKEN),)
export DEFECTDOJO_USERNAME ?= admin
export DEFECTDOJO_PASSWORD ?= DefectDojoAcc1!
endif

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./... $(TESTARGS)

.PHONY: fmt lint test testacc build install generate up down
