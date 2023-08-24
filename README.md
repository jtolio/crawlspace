# crawlspace

https://pkg.go.dev/github.com/jtolio/crawlspace

package crawlspace provides a means to dynamically interact with registered Go
objects in a live process, using a github.com/mattn/anko shell.

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
	crawlspace.RegisterType("MyType", MyType{})
	panic(crawlspace.ListenAndServe(2222))
}
```

After running the above program, you can now connect via telnet or netcat
to localhost:2222, and run the following interaction:

```
> x = make(MyType)
> print(x.Get())
0
> x.Set(5)
> print(x.Get())
5
```

Copyright 2015-2023, JT Olds. Licensed under Apache License 2.0
