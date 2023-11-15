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
> filter(packages(), "net")
[]string{"net", "net/http"}

> import "net/http"
> dir(http)
[]string{"Get", "DefaultClient", ...}

> (*try.E1(net.LookupIP("google.com")))[0].String()
"2607:f8b0:4009:81c::200e"

> (*try.E1(net.LookupIP("google.com")))[1].String()
"142.251.32.14"

> newAt("net", "conn", 0x40003711b8).RemoteAddr().String()
"127.0.0.1:57538"

> dir()
[]string{"call", "catch", "debugServer", "def", "dir", "false", "filter", "functions", "global", "globals", "len", "mut", "newAt", "nil", "packages", "pretty", "printf", "println", "quit", "sudo", "true", "try", "types"}

> x := newAt("net", "conn", 0x40001321c0)
> x
&net.conn{fd:(*net.netFD)(0x4000598000)}

> dir(x)
[]string{"Close", "File", "LocalAddr", "Read", "RemoteAddr", "SetDeadline", "SetReadBuffer", "SetReadDeadline", "SetWriteBuffer", "SetWriteDeadline", "Write", "fd"}

> x.LocalAddr()
(*net.TCPAddr)(0x400057e300)
> x.LocalAddr().String()
"127.0.0.1:7778"
```

Copyright 2015-2023, JT Olds. Licensed under Apache License 2.0
