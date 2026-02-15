::go install github.com/akavel/rsrc@latest
::go mod tidy

::rsrc -manifest main.manifest -o main.syso
::rsrc -manifest main.manifest -ico icon.ico -o main.syso
rsrc -manifest main.manifest -ico icon.ico

:: Build under Windows for Windows
go build -trimpath -gcflags "-l -B" -ldflags="-s -w" -o MSIAfterburnerProfileSwitcherDebug.exe
go build -trimpath -gcflags "-l -B" -ldflags="-s -w -H windowsgui" -o MSIAfterburnerProfileSwitcher.exe

:: Build under Linux for Windows
::GOOS=windows GOARCH=amd64 go build -trimpath -gcflags "-l -B" -ldflags="-s -w" -o MSIAfterburnerProfileSwitcherDebug.exe
::GOOS=windows GOARCH=amd64 go build -trimpath -gcflags "-l -B" -ldflags="-s -w -H windowsgui" -o MSIAfterburnerProfileSwitcher.exe
