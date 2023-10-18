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
objects in a live process, using small scripting language based around the
reflect package.

Inspiration is mainly from Twisted's manhole library:
https://twistedmatrix.com/documents/current/api/twisted.conch.manhole.html
*/
package crawlspace

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"time"

	"github.com/jtolio/crawlspace/reflectlang"
)

// Crawlspace is a registry of Go values to expose via a remote shell.
type Crawlspace struct {
	env func(out io.Writer) reflectlang.Environment
}

// New makes a new crawlspace using the environment constructor env.
// If env is nil, reflectlang.Environment{} is used.
// github.com/jtolio/crawlspace/tools.Env is perhaps a more useful choice.
func New(env func(out io.Writer) reflectlang.Environment) *Crawlspace {
	if env == nil {
		env = func(io.Writer) reflectlang.Environment { return reflectlang.Environment{} }
	}
	return &Crawlspace{env: env}
}

// Interact takes input from `in` and returns output to `out`. It runs until
// there is an error, or the user runs `quit()`. In the case of the input
// returning io.EOF or the user entering `quit()`, no error will be returned.
func (m *Crawlspace) Interact(in io.Reader, out io.Writer) error {
	_, err := fmt.Fprintf(out, "%s\n%s\n", crawlspaceVersion, processVersion)
	if err != nil {
		return err
	}

	env := m.env(out)
	eof := false
	env["quit"] = reflect.ValueOf(func() { eof = true })
	var lastvals []reflect.Value
	env["_"] = reflectlang.LowerFunc(env, func(args []reflect.Value) ([]reflect.Value, error) {
		if len(args) != 0 {
			return nil, fmt.Errorf("unexpected argument")
		}
		return lastvals, nil
	})

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
		rv, err := reflectlang.Eval(line, env)
		if err != nil {
			_, err = fmt.Fprintf(out, "%v\n", err)
			if err != nil {
				return err
			}
			continue
		}
		lastvals = rv
		for _, val := range rv {
			_, err = fmt.Fprintf(out, "%#v\n", val)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// ListenAndServe listens on the given address. It calls Serve with an
// appropriate listener.
func (m *Crawlspace) ListenAndServe(addr string) error {
	l, err := net.Listen("tcp", addr)
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
