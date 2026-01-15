.PHONY: fmt fmt-check lint typecheck test vuln ci

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

lint: fmt-check
	go vet ./...

typecheck:
	go list ./... | xargs go test -run=^$

test:
	go test ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

ci: lint typecheck test vuln
