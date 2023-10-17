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
> print(x.Get())
0
> x.Set(5)
> print(x.Get())
5
```

If you import the `github.com/jtolds/crawlspace/tools`, you can have a better
experience.

```
	space := crawlspace.New(tools.Env)
```

And here's an example history inspecting a process:

```
> filter(packages(), "storj.io/uplink")
[]string{"storj.io/uplink", "storj.io/uplink/private/ecclient", "storj.io/uplink/private/eestream", "storj.io/uplink/private/eestream/scheduler", "storj.io/uplink/private/metaclient", "storj.io/uplink/private/multipart", "storj.io/uplink/private/piecestore", "storj.io/uplink/private/storage/streams", "storj.io/uplink/private/storage/streams/batchaggregator", "storj.io/uplink/private/storage/streams/buffer", "storj.io/uplink/private/storage/streams/pieceupload", "storj.io/uplink/private/storage/streams/segmenttracker", "storj.io/uplink/private/storage/streams/segmentupload", "storj.io/uplink/private/storage/streams/splitter", "storj.io/uplink/private/storage/streams/streambatcher", "storj.io/uplink/private/storage/streams/streamupload", "storj.io/uplink/private/stream", "storj.io/uplink/private/testuplink", "storj.io/uplink/private/version"}

> globals("storj.io/uplink")
[]string{"ErrBandwidthLimitExceeded", "ErrBucketAlreadyExists", "ErrBucketNameInvalid", "ErrBucketNotEmpty", "ErrBucketNotFound", "ErrObjectKeyInvalid", "ErrObjectNotFound", "ErrPermissionDenied", "ErrSegmentsLimitExceeded", "ErrStorageLimitExceeded", "ErrTooManyRequests", "ErrUploadDone", "ErrUploadIDInvalid", "maxSegmentSize", "noiseVersion", "packageError"}

> filter(functions("storj.io/uplink"), "*Access")
[]string{"(*Access).SatelliteAddress", "(*Access).Serialize"}

> (*try.E1(call("net", "LookupIP", "google.com")))[0].String()
"2607:f8b0:4009:81c::200e"

> (*try.E1(call("net", "LookupIP", "google.com")))[1].String()
"142.251.32.14"

> newAt("net", "conn", 0x40003711b8).RemoteAddr().String()
"127.0.0.1:57538"

> dir()
[]string{"call", "catch", "debugServer", "def", "dir", "false", "filter", "functions", "global", "globals", "len", "mut", "newAt", "nil", "packages", "pretty", "printf", "println", "quit", "sudo", "true", "try", "types"}

> def("x", newAt("net", "conn", 0x40001321c0))
> x
&net.conn{fd:(*net.netFD)(0x4000598000)}

> dir(x)
[]string{"Close", "File", "LocalAddr", "Read", "RemoteAddr", "SetDeadline", "SetReadBuffer", "SetReadDeadline", "SetWriteBuffer", "SetWriteDeadline", "Write", "fd"}

> x.LocalAddr()
(*net.TCPAddr)(0x400057e300)
> x.LocalAddr().String()
"127.0.0.1:7778"

> def("monkit", debugServer.baseRegistry)
> dir(monkit)
panic: reflect: reflect.Value.Set using value obtained using unexported field
> dir(sudo(monkit))
[]string{"AllSpans", "Funcs", "ObserveTraces", "Package", "RootSpans", "ScopeNamed", "Scopes", "Stats", "WithTransformers", "registryInternal", "transformers"}

> sudo(monkit).ScopeNamed("storj.io/private/process").sources["func:root"]
(*monkit.Func)(0x40001d7500)
> dir(sudo(monkit).ScopeNamed("storj.io/private/process").sources["func:root"])
panic: reflect: reflect.Value.Set using value obtained using unexported field
> dir(sudo(sudo(monkit).ScopeNamed("storj.io/private/process").sources["func:root"]))
[]string{"Current", "Errors", "FailureTimes", "FullName", "FuncStats", "Highwater", "Id", "Observe", "Panics", "Parents", "RemoteTrace", "Reset", "ResetTrace", "Scope", "ShortName", "Stats", "Success", "SuccessTimes", "Task", "id", "key", "scope"}
```

Copyright 2015-2023, JT Olds. Licensed under Apache License 2.0
