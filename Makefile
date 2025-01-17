BASE_PATH := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
MKFILE_PATH := $(BASE_PATH)/Makefile
COVER_OUT := cover.out

.DEFAULT_GOAL := help

test: ## Run tests
	go test -v -count=1 ./... -coverprofile=$(COVER_OUT)

bench: ## Run benchmarks
	go test -benchmem -bench .

cover: ## Show tests coverage
	@if [ -f $(COVER_OUT) ]; then \
		echo "Coverage:" \
		go tool cover -func=$(COVER_OUT); \
		rm -f $(COVER_OUT); \
	else \
		echo "$(COVER_OUT) is missing. Please run 'make test'"; \
	fi

clean: ## Clean up
	@rm -rf bin
	@rm -f $(COVER_OUT)
	@find $(BASE_PATH) -name ".DS_Store" -depth -exec rm {} \;

help: ## Show help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: all \
        test bench cover \
        clean help
