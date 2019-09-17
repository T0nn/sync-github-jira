
GO        := GO111MODULE=on go
GOBUILD   := CGO_ENABLED=0 $(GO) build

LDFLAGS += -X "main.BuildTS=$(shell date -u '+%Y-%m-%d %I:%M:%S')"
LDFLAGS += -X "main.GitHash=$(shell git rev-parse HEAD)"
LDFLAGS += -X "main.GitBranch=$(shell git rev-parse --abbrev-ref HEAD)"


.PHONY: all build

default: build

build:
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/sync-jira