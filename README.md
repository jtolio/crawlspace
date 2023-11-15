# crawlspace

https://pkg.go.dev/github.com/jtolio/crawlspace

package crawlspace provides a means to dynamically interact with registered Go
objects in a live process, using small scripting language based around the
reflect package.

Inspiration is mainly from Twisted's manhole library:
https://twistedmatrix.com/documents/current/api/twisted.conch.manhole.html

Example usage:

```
package main

import (
	"github.com/jtolds/crawlspace"
)

type MyType struct{ x int }

func (m *MyType) Set(x int) { m.x = x }
func (m *MyType) Get() int  { return m.x }

func main() {
	space := crawlspace.New(nil)
	space.RegisterVal("x", &MyType{})
	panic(crawlspace.ListenAndServe("localhost:2222"))
}
```

After running the above program, you can now connect via telnet or netcat
to localhost:2222, and run the following interaction:

```
> x.Get()
0
> x.Set(5)
> x.Get()
5
```

If you import the `github.com/jtolds/crawlspace/tools`, you can have a better
experience.

```
	space := crawlspace.New(tools.Env)
```

And here's an example history inspecting a process:

```
~/dev/crawlspace-test$ telnet localhost 2222
Trying 127.0.0.1...
Connected to localhost.
Escape character is '^]'.
github.com/jtolio/crawlspace@v0.0.0-20231013070742-9283b10c8cf6
github.com/jtolio/crawlspace-test@(devel)
> import "net"
> conn := reflect.NewAt(net.conn, unsafe.Pointer(0xc0000440d0)).Interface()
> conn
(*net.conn)(0xc0000440d0)
> dir(conn)
[]string{"Close", "File", "LocalAddr", "Read", "RemoteAddr", "SetDeadline", "SetReadBuffer", "SetReadDeadline", "SetWriteBuffer", "SetWriteDeadline", "Write", "fd"}
> conn.RemoteAddr()
(*net.TCPAddr)(0xc00007fcb0)
> addr := conn.RemoteAddr()
> addr.String()
"127.0.0.1:38880"
> dir(addr)
[]string{"AddrPort", "IP", "Network", "Port", "String", "Zone"}
> ips, err := net.LookupIP("google.com")
> ips[1]
net.IP{0xac, 0xd9, 0x1, 0x6e}
> ips[1].String()
"172.217.1.110"
>
```

Copyright 2015-2023, JT Olds. Licensed under Apache License 2.0
