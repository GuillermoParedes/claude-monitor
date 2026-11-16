VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build windows test vet

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o dist/claude-monitor ./cmd/claude-monitor

windows:
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS) -H windowsgui" -o dist/claude-monitor_windows_amd64.exe ./cmd/claude-monitor

test:
	go test ./...

vet:
	go vet ./... && GOOS=windows go vet ./...
