go install github.com/akavel/rsrc@latest
go mod tidy
rsrc -manifest main.manifest -ico trayicon/icon.ico
go build -trimpath -gcflags "-l -B" -ldflags="-s -w" -o MSIAfterburnerProfileSwitcher.exe
