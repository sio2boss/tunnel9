VERSION := v$(shell ./tools/version.sh)
APP := tunnel9

.PHONY: all release homebrew test tests test-unit test-coverage clean

all:
	go build -trimpath

release: linux_arm64 linux_amd64 apple_amd64 apple_arm64 win_amd64 win_arm64 update_version

homebrew: release
	./tools/update_homebrew_formula.sh

linux_arm64:
	mkdir -p release
	env GOOS=linux GOARCH=arm64 go build -trimpath
	tar cvfz release/${APP}-$(VERSION)-linux-arm64.tar.gz ${APP}
	rm -f ${APP}

linux_amd64:
	mkdir -p release
	env GOOS=linux GOARCH=amd64 go build -trimpath
	tar cvfz release/${APP}-$(VERSION)-linux-amd64.tar.gz ${APP}
	rm -f ${APP}

apple_amd64:
	mkdir -p release
	env GOOS=darwin GOARCH=amd64 go build -trimpath
	tar cvfz release/${APP}-$(VERSION)-apple-amd64.tar.gz ${APP}
	rm -f ${APP}

apple_arm64:
	mkdir -p release
	env GOOS=darwin GOARCH=arm64 go build -trimpath
	tar cvfz release/${APP}-$(VERSION)-apple-arm64.tar.gz ${APP}
	rm -f ${APP}

win_amd64:
	mkdir -p release
	env GOOS=windows GOARCH=amd64 go build -trimpath
	zip release/${APP}-$(VERSION)-windows-amd64.zip ${APP}.exe
	rm -f ${APP}.exe

win_arm64:
	mkdir -p release
	env GOOS=windows GOARCH=arm64 go build -trimpath
	zip release/${APP}-$(VERSION)-windows-arm64.zip ${APP}.exe
	rm -f ${APP}.exe

update_version:
	sed -i .bak "s/VERSION=.*/VERSION=$(VERSION)/1" tools/install.sh

vhs: all
	cd docs && vhs tui.tape

# Test targets
test: test-unit

tests: test-unit

test-unit:
	go test -v ./internal/...

test-coverage:
	go test -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-watch:
	@echo "Starting test watcher (requires gotestsum)..."
	gotestsum --watch ./internal/...
