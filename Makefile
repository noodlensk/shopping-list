setup-lint: ## Set up linter
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.47.2
fmt: ## gofmt and goimports all go files
	find . -name '*.go' | while read -r file; do gofmt -w -s "$$file"; goimports -w "$$file"; done
dep: ## Get all dependencies
	go mod download
	go mod tidy -compat=1.17
build: ## Build binary
	go build
run: build ## Run
	./shopping-list -config config.yaml
lint: ## Lint
	golangci-lint run
test: ## Test
	go test -count=1 -p=8 -parallel=8 -race ./...
lint-fix: ## Lint fix
	golangci-lint run --fix
start-env: ## Start local env
	docker-compose up -d
stop-env: ## Stop local env
	docker-compose down
# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help