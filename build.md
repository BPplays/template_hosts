
```pwsh
go build -buildvcs=false; sudo .\template_hosts.exe -service stop; sudo .\template_hosts.exe -service uninstall; sudo .\template_hosts.exe -service install; sudo .\template_hosts.exe -service start
```
