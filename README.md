# vnc-client
[PoC] VNC Client

## Dependency
- sdl2
- go

## How to use

```
$ ssh -L 5900:localhost:5900 server
```

```
$ go mod tidy
$ go build
$ ./vnc-client
Password: <password>
```
