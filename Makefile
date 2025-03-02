VERSION := v$(shell ./tools/version.sh)
APP := tunnel9

.PHONY: all release

all:
	go build -trimpath

release: linux_arm64 linux_amd64 apple_amd64 apple_arm64 win_amd64 win_arm64 update_version

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

clean:
	rm -f ./tunnel9
	rm -rf ./release
	rm -f ./go.sum

install: all
	cp ./tunnel9 ~/.local/bin/

vhs: all
	cd docs && vhs tui.tape
