# go-manhole

https://godoc.org/github.com/jtolds/go-manhole

package manhole provides a means to dynamically interact with registered Go
objects in a live process, using a Lua shell.

Inspiration is mainly from Twisted's library of the same name:
https://twistedmatrix.com/documents/current/api/twisted.conch.manhole.html

Example usage:

```
package main

import (
  "github.com/jtolds/go-manhole"
)

type MyType struct{ x int }

func (m *MyType) Set(x int) { m.x = x }
func (m *MyType) Get() int  { return m.x }

func main() {
  manhole.RegisterType("MyType", MyType{})
  panic(manhole.ListenAndServe(2222))
}
```

After running the above program, you can now connect via telnet or netcat
to localhost:2222, and run the following interaction:

```
> x = MyType()
> print(x:Get())
0
> x:Set(5)
> print(x:Get())
5
```

Copyright 2015, JT Olds. Licensed under Apache License 2.0
