# Yet Another VNC Client [EXPERIMENTAL]

## Supported

- Compression With Tight Encoding
- JPEG Compression
- Desktop Size Adjustment
- Clipboard (WIP)

## Dependency
- [Simple DirectMedia Layer](https://www.libsdl.org/)
- [Go](golang.org)
- [go-zlib](https://github.com/4kills/go-zlib/blob/master/LICENSE)
- [go-sdl2](https://github.com/veandco/go-sdl2)

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

## See also

- [The Remote Framebuffer Protocol](https://datatracker.ietf.org/doc/html/rfc6143)
- [VNC/RFB Protocol Specification](https://github.com/rfbproto/rfbproto)
