.PHONY: dev server web generate test vet lint typecheck web-test build check

GO_PACKAGES := ./cmd/... ./internal/... ./ent/...

dev:
	@echo "Run 'make server' and 'make web' in separate terminals."

server:
	go run ./cmd/novro

web:
	pnpm --dir apps/web dev

generate:
	go generate ./ent

test:
	go test $(GO_PACKAGES)

vet:
	go vet $(GO_PACKAGES)

lint:
	pnpm --dir apps/web lint

typecheck:
	pnpm --dir apps/web typecheck

web-test:
	pnpm --dir apps/web test

build:
	go build -o bin/novro.exe ./cmd/novro
	pnpm --dir apps/web build

check: test vet lint typecheck web-test build
