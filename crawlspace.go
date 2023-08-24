// Copyright 2015-2023 JT Olds
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

/*
package crawlspace provides a means to dynamically interact with registered Go
objects in a live process, using a Lua shell.

Inspiration is mainly from Twisted's manhole library:
https://twistedmatrix.com/documents/current/api/twisted.conch.manhole.html

Example usage:

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

After running the above program, you can now connect via telnet or netcat
to localhost:2222, and run the following interaction:

	> x = MyType.new()
	> print(x.Get())
	0
	> x.Set(5)
	> print(x.Get())
	5
*/
package crawlspace

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Shopify/go-lua"
	"github.com/jtolds/go-luar"
)

var (
	reserved = map[string]bool{"quit": true, "print": true, "repr": true}
)

// Crawlspace is essentially a registry of Go values to expose via a remote shell.
type Crawlspace struct {
	mtx           sync.Mutex
	registrations map[string]func(*lua.State) error
}

func (m *Crawlspace) register(name string, val interface{},
	pusher func(l *lua.State, val interface{}) error) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.registrations == nil {
		m.registrations = map[string]func(*lua.State) error{}
	}

	_, exists := m.registrations[name]
	if exists || reserved[name] {
		return fmt.Errorf("Registration %#v already exists", name)
	}
	m.registrations[name] = func(l *lua.State) error {
		err := pusher(l, val)
		if err != nil {
			return err
		}
		l.SetGlobal(name)
		return nil
	}
	return nil
}

// RegisterType registers the type with example value `example` using the
// global name `name`.
// Example:
//
//	m.RegisterType("MyType", MyType{})
//
// Applies to all future crawlspace sessions, but not already started ones.
func (m *Crawlspace) RegisterType(name string, example interface{}) error {
	return m.register(name, example, luar.PushType)
}

// RegisterVal registers the value `value` using the global name `name`.
// Example:
//
//	m.RegisterVal("x", x)
//
// Applies to all future crawlspace sessions, but not already started ones.
func (m *Crawlspace) RegisterVal(name string, value interface{}) error {
	return m.register(name, value, luar.PushValue)
}

// Unregister removes the previously-registered global name `name` from all
// future crawlspace sessions.
func (m *Crawlspace) Unregister(name string) {
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
func (m *Crawlspace) Interact(in io.Reader, out io.Writer) error {
	l := lua.NewState()
	luar.SetOptions(l, luar.Options{AllowUnexportedAccess: true})

	m.mtx.Lock()
	names := make([]string, 0, len(m.registrations)+len(reserved))
	registrations := make([]func(l *lua.State) error, 0, len(m.registrations))
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
		err := reg(l)
		if err != nil {
			return err
		}
	}

	eof := false
	err := luar.PushValue(l, func() { eof = true })
	if err != nil {
		return err
	}
	l.SetGlobal("quit")

	err = luar.PushValue(l, func(vals ...interface{}) {
		fmt.Fprintln(out, vals...)
	})
	if err != nil {
		return err
	}
	l.SetGlobal("print")

	err = luar.PushValue(l, func(vals ...interface{}) {
		for i, val := range vals {
			fmt.Fprintf(out, "%#v", val)
			if i > 0 {
				fmt.Fprintf(out, " ")
			}
		}
		fmt.Fprintln(out)
	})
	if err != nil {
		return err
	}
	l.SetGlobal("repr")

	_, err = fmt.Fprintf(out, "crawlspace registrations:\n%s\n",
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
		var line string
		for {
			line, err = stdin.ReadString('\n')
			eof = errors.Is(err, io.EOF)
			line = strings.TrimSpace(line)
			empty := len(line) == 0
			if err != nil && (!eof || empty) {
				return err
			}
			if !empty {
				break
			}
		}
		err = lua.DoString(l, line)
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
func (m *Crawlspace) ListenAndServe(port int) error {
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return err
	}
	return m.Serve(l)
}

// Serve accepts incoming connections and calls Interact with both sides of
// incoming client connections. Careful, it's probably a security mistake to
// use a listener that can accept connections from anywhere.
func (m *Crawlspace) Serve(l net.Listener) error {
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
			m.Interact(&eotTranslate{conn}, conn)
		}()
	}
}

var (
	Default = &Crawlspace{}

	RegisterType   = Default.RegisterType
	RegisterVal    = Default.RegisterVal
	Unregister     = Default.Unregister
	Interact       = Default.Interact
	Serve          = Default.Serve
	ListenAndServe = Default.ListenAndServe
)

type eotTranslate struct {
	data io.Reader
}

const asciiEOT = 0x04

func (w *eotTranslate) Read(p []byte) (n int, err error) {
	n, err = w.data.Read(p)
	if err == nil && n > 0 && p[n-1] == asciiEOT {
		err = io.EOF
		n--
	}
	return n, err
}
