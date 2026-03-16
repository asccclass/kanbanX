

buildWin:
	set CGO_ENABLED=0&& set GOOS=windows&& set GOARCH=amd64&& go build -o kanbanX.exe .

buildMac:
	set CGO_ENABLED=0&& set GOOS=darwin&& set GOARCH=amd64&& go build -o kanbanX .

buildLinux:
	set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=amd64&& go build -o kanbanX .

buildARM:
	set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=arm64&& go build -o kanbanX .

s:
	git push -u origin main