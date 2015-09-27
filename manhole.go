/*
package manhole provides a means to dynamically interact with registered Go
objects in a live process, using a Lua shell.

Inspiration is mainly from Twisted's library of the same name:
https://twistedmatrix.com/documents/current/api/twisted.conch.manhole.html

Example usage:

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

After running the above program, you can now connect via telnet or netcat
to localhost:2222, and run the following interaction:

    > x = MyType()
    > print(x:Get())
    0
    > x:Set(5)
    > print(x:Get())
    5

*/
package manhole

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/layeh/gopher-luar"
	"github.com/yuin/gopher-lua"
)

var (
	reserved = map[string]bool{"quit": true, "print": true, "repr": true}
)

// Manhole is essentially a registry of Go values to expose via a remote shell.
type Manhole struct {
	mtx           sync.Mutex
	registrations map[string]func(*lua.LState)
}

func (m *Manhole) register(name string, val interface{},
	mapper func(l *lua.LState, val interface{}) lua.LValue) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.registrations == nil {
		m.registrations = map[string]func(*lua.LState){}
	}

	_, exists := m.registrations[name]
	if exists || reserved[name] {
		return fmt.Errorf("Registration %#v already exists", name)
	}
	m.registrations[name] = func(l *lua.LState) {
		l.SetGlobal(name, mapper(l, val))
	}
	return nil
}

// RegisterType registers the type with example value `example` using the
// global name `name`.
// Example:
//   m.RegisterType("MyType", MyType{})
// Applies to all future manhole sessions, but not already started ones.
func (m *Manhole) RegisterType(name string, example interface{}) error {
	return m.register(name, example, luar.NewType)
}

// RegisterVal registers the value `value` using the global name `name`.
// Example:
//   m.RegisterVal("x", x)
// Applies to all future manhole sessions, but not already started ones.
func (m *Manhole) RegisterVal(name string, value interface{}) error {
	return m.register(name, value, luar.New)
}

// Unregister removes the previously-registered global name `name` from all
// future manhole sessions.
func (m *Manhole) Unregister(name string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.registrations == nil {
		return
	}
	delete(m.registrations, name)
}

// Interact takes input from `in` and returns output to `out`. It runs until
// there is an error, or the user runs `quit()`. In the case of the input
// returning io.EOF or the user entering `quit()`, no error will be returned.
func (m *Manhole) Interact(in io.Reader, out io.Writer) error {
	l := lua.NewState(lua.Options{
		SkipOpenLibs: true})
	defer l.Close()

	m.mtx.Lock()
	names := make([]string, 0, len(m.registrations)+len(reserved))
	registrations := make([]func(l *lua.LState), 0, len(m.registrations))
	for name, reg := range m.registrations {
		names = append(names, name)
		registrations = append(registrations, reg)
	}
	m.mtx.Unlock()
	for name := range reserved {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, reg := range registrations {
		reg(l)
	}

	eof := false
	l.SetGlobal("quit", luar.New(l, func() { eof = true }))
	l.SetGlobal("print", luar.New(l, func(vals ...interface{}) {
		fmt.Fprintln(out, vals...)
	}))
	l.SetGlobal("repr", luar.New(l, func(vals ...interface{}) {
		for i, val := range vals {
			fmt.Fprintf(out, "%#v", val)
			if i > 0 {
				fmt.Fprintf(out, " ")
			}
		}
		fmt.Fprintln(out)
	}))

	_, err := fmt.Fprintf(out, "go-manhole registrations:\n%s\n",
		strings.Join(names, ", "))
	if err != nil {
		return err
	}

	stdin := bufio.NewReader(in)
	for !eof {
		_, err := fmt.Fprintf(out, "> ")
		if err != nil {
			return err
		}
		line, err := stdin.ReadString('\n')
		eof = err == io.EOF
		if err != nil && !eof {
			return err
		}
		err = l.DoString(line)
		if err != nil {
			_, err = fmt.Fprintf(out, "%v\n", err)
			if err != nil {
				return err
			}
			continue
		}
	}
	return nil
}

// ListenAndServe listens on localhost with the given port. It calls Serve
// with an appropriate listener
func (m *Manhole) ListenAndServe(port int) error {
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return err
	}
	return m.Serve(l)
}

// Serve accepts incoming connections and calls Interact with both sides of
// incoming client connections. Careful, it's probably a security mistake to
// use a listener that can accept connections from anywhere.
func (m *Manhole) Serve(l net.Listener) error {
	defer l.Close()
	var delay time.Duration
	for {
		conn, err := l.Accept()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				if delay == 0 {
					delay = 5 * time.Millisecond
				} else {
					delay *= 2
				}
				if delay > time.Second {
					delay = time.Second
				}
				time.Sleep(delay)
				continue
			}
			return err
		}
		delay = 0
		go func() {
			defer conn.Close()
			m.Interact(conn, conn)
		}()
	}
}

var (
	Default = &Manhole{}

	RegisterType   = Default.RegisterType
	RegisterVal    = Default.RegisterVal
	Unregister     = Default.Unregister
	Interact       = Default.Interact
	Serve          = Default.Serve
	ListenAndServe = Default.ListenAndServe
)
