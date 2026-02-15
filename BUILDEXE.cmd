go install github.com/akavel/rsrc@latest
go mod tidy
rsrc -manifest main.manifest -ico icon.ico
go build -trimpath -gcflags "-l -B" -ldflags="-s -w" -o MSIAfterburnerProfileSwitcherDebug.exe
go build -trimpath -gcflags "-l -B" -ldflags="-s -w -H windowsgui" -o MSIAfterburnerProfileSwitcher.exe
