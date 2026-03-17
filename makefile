# KanbanX Makefile
# Works on Linux, macOS, and Windows (with Git Bash / PowerShell)

APP     = kanbanX
PORT    = 8080

.PHONY: run build clean vendor tidy

## run: start the dev server (auto-downloads deps if needed)
run:
	go run .

## build: compile a self-contained binary (no CGO, no C compiler needed)
build:
	CGO_ENABLED=0 go build -o $(APP) .

## build-windows: compile for Windows
build-windows:
	set CGO_ENABLED=0&& set GOOS=windows&& set GOARCH=amd64&& go build -o $(APP).exe .


buildMac:
	set CGO_ENABLED=0&& set GOOS=darwin&& set GOARCH=amd64&& go build -o $(APP) .

buildLinux:
	set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=amd64&& go build -o $(APP) .

buildARM:
	set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=arm64&& go build -o $(APP) .

## vendor: update the vendor/ directory
vendor:
	GOPROXY=direct GONOSUMDB='*' go mod vendor

## tidy: tidy go.mod / go.sum
tidy:
	GOPROXY=direct GONOSUMDB='*' go mod tidy

## clean: remove build artifacts and database
clean:
	rm -f $(APP) $(APP).exe kanban.db

help:
	@grep -E '^## ' Makefile | sed 's/## //'

s:
	git push -u origin main