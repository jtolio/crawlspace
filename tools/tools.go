package tools

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unsafe"

	"github.com/jtolio/crawlspace/reflectlang"
	"github.com/kr/pretty"
	"github.com/zeebo/goof"
	"github.com/zeebo/sudo"
)

var troop goof.Troop

func assert(err error) {
	if err != nil {
		panic(err)
	}
}

func Env(out io.Writer) reflectlang.Environment {
	env := reflectlang.NewStandardEnvironment()

	env["$forcedImports"] = reflect.ValueOf(func() []interface{} {
		return []interface{}{
			reflect.NewAt,
			reflect.TypeOf(unsafe.Pointer(nil)),
			pretty.Sprint,
		}
	})

	env["try"] = reflectlang.LowerStruct(env, reflectlang.Environment{
		"E": reflect.ValueOf(assert),
		"E1": reflect.ValueOf(func(a interface{}, err error) (_ interface{}) {
			assert(err)
			return a
		}),
		"E2": reflect.ValueOf(func(a, b interface{}, err error) (_, _ interface{}) {
			assert(err)
			return a, b
		}),
		"E3": reflect.ValueOf(func(a, b, c interface{}, err error) (_, _, _ interface{}) {
			assert(err)
			return a, b, c
		}),
		"E4": reflect.ValueOf(func(a, b, c, d interface{}, err error) (_, _, _, _ interface{}) {
			assert(err)
			return a, b, c, d
		}),
	})

	env["int64"] = reflect.ValueOf(reflect.TypeOf(int64(0)))
	env["uint64"] = reflect.ValueOf(reflect.TypeOf(uint64(0)))
	env["int"] = reflect.ValueOf(reflect.TypeOf(int(0)))
	env["uint"] = reflect.ValueOf(reflect.TypeOf(uint(0)))
	env["uintptr"] = reflect.ValueOf(reflect.TypeOf(uintptr(0)))
	env["int32"] = reflect.ValueOf(reflect.TypeOf(int32(0)))
	env["uint32"] = reflect.ValueOf(reflect.TypeOf(uint32(0)))
	env["float32"] = reflect.ValueOf(reflect.TypeOf(float32(0)))
	env["float64"] = reflect.ValueOf(reflect.TypeOf(float64(0)))
	env["string"] = reflect.ValueOf(reflect.TypeOf(string("")))
	env["byte"] = reflect.ValueOf(reflect.TypeOf(byte(0)))

	env["packages"] = reflect.ValueOf(func(contains ...string) []string {
		pkgs := map[string]bool{}
		process := func(names []string) {
			for _, name := range names {
				if strings.HasPrefix(name, "go:") ||
					strings.HasPrefix(name, "struct {") {
					continue
				}
				name = strings.TrimPrefix(name, "type:.eq.")
				name = strings.TrimPrefix(name, "type:.hash.")
				lastSlash := strings.LastIndex(name, "/")
				pkgPrefix := ""
				if lastSlash >= 0 {
					pkgPrefix = name[:lastSlash]
					name = name[lastSlash:]
				}

				pos := strings.Index(name, ".")
				if pos < 0 {
					pkgs[pkgPrefix] = true
					continue
				}
				pkgs[pkgPrefix+name[:pos]] = true
			}
		}

		names, err := troop.Globals()
		assert(err)
		process(names)

		names, err = troop.Functions()
		assert(err)
		process(names)

		types, err := troop.Types()
		assert(err)
		for _, typ := range types {
			pkgs[typ.PkgPath()] = true
		}

		names = make([]string, 0, len(pkgs))
		for pkg := range pkgs {
			okayToAdd := true
			for _, needle := range contains {
				if !strings.Contains(pkg, needle) {
					okayToAdd = false
					break
				}
			}
			if okayToAdd {
				names = append(names, pkg)
			}
		}
		sort.Strings(names)
		return names
	})

	topLevelDirSuppressions := map[string]reflect.Value{}
	for _, name := range []string{
		"byte", "false", "float32", "float64", "int", "int32", "int64", "len",
		"nil", "string", "true", "uint", "uint32", "uint64", "uintptr"} {
		topLevelDirSuppressions[name] = env[name]
	}

	env["dir"] = reflect.ValueOf(func(args ...interface{}) []string {
		handleEnv := func(sub reflectlang.Environment, isEnv bool) []string {
			names := []string{}
			for key, val := range sub {
				if isEnv && val == topLevelDirSuppressions[key] {
					continue
				}
				if !strings.HasPrefix(key, "$") {
					names = append(names, key)
				}
			}
			sort.Strings(names)
			return names
		}
		if len(args) == 0 {
			return handleEnv(env, true)
		}

		if sub := reflectlang.IsLowerStruct(args[0]); sub != nil {
			return handleEnv(sub, false)
		}
		if reflectlang.IsLowerFunc(args[0]) {
			return []string{}
		}

		fields := []string{}
		handle := func(typ reflect.Type) {
			for i := 0; i < typ.NumMethod(); i++ {
				fields = append(fields, typ.Method(i).Name)
			}
			if typ.Kind() == reflect.Struct {
				for i := 0; i < typ.NumField(); i++ {
					fields = append(fields, typ.Field(i).Name)
				}
			}
		}

		arg := args[0]
		typ := reflect.TypeOf(arg)
		handle(typ)
		if typ.Kind() == reflect.Pointer {
			handle(typ.Elem())
		}
		sort.Strings(fields)
		return fields
	})

	env["println"] = reflect.ValueOf(func(args ...interface{}) {
		_, err := fmt.Fprintln(out, args...)
		assert(err)
	})

	env["printf"] = reflect.ValueOf(func(msgf string, args ...interface{}) {
		_, err := fmt.Fprintf(out, msgf, args...)
		assert(err)
	})

	env["sudo"] = reflectlang.LowerFunc(env, func(args []reflect.Value) ([]reflect.Value, error) {
		result := make([]reflect.Value, 0, len(args))
		for _, arg := range args {
			result = append(result, sudo.Sudo(arg))
		}
		return result, nil
	})

	env["$import"] = reflectlang.LowerFunc(env, func(args []reflect.Value) ([]reflect.Value, error) {

		if len(args) != 2 {
			return nil, fmt.Errorf("import expected 2 arguments")
		}
		if args[0].Kind() != reflect.String {
			return nil, fmt.Errorf("import expected a target name argument")
		}
		if args[1].Kind() != reflect.String {
			return nil, fmt.Errorf("import expected a package name")
		}

		target := args[0].String()
		pkgName := args[1].String()

		if target == "_" {
			return nil, nil
		}
		var envToFill reflectlang.Environment
		if target == "." {
			envToFill = env
		} else {
			if target == "" {
				target = importPathToNameBasic(pkgName)
			}
			envToFill = reflectlang.Environment{}
		}

		types, err := troop.Types()
		if err != nil {
			return nil, err
		}
		for _, typ := range types {
			if typ.PkgPath() == pkgName {
				envToFill[typ.Name()] = reflect.ValueOf(typ)
			}
		}

		scanList := func(names []string, loader func(name string) (reflect.Value, error)) error {
			for _, name := range names {
				if !strings.HasPrefix(name, pkgName+".") {
					continue
				}
				localName := strings.TrimPrefix(name, pkgName+".")
				if !reflectlang.IsIdentifier(localName) {
					continue
				}
				global, err := loader(name)
				if err != nil {
					return err
				}
				envToFill[localName] = global
			}
			return nil
		}

		globals, err := troop.Globals()
		if err != nil {
			return nil, err
		}
		if err = scanList(globals, troop.Global); err != nil {
			return nil, err
		}

		functions, err := troop.Functions()
		if err != nil {
			return nil, err
		}
		if err = scanList(functions, func(name string) (reflect.Value, error) {
			return reflectlang.LowerFunc(env, func(args []reflect.Value) (_ []reflect.Value, err error) {
				iargs := make([]interface{}, 0, len(args))
				for _, arg := range args {
					// TODO: can we leave these reflect.Values?
					iargs = append(iargs, arg.Interface())
				}

				results, err := troop.Call(name, iargs...)
				if err != nil {
					return nil, err
				}

				var iresults []reflect.Value
				for _, res := range results {
					iresults = append(iresults, reflect.ValueOf(res))
				}

				return iresults, nil
			}), nil
		}); err != nil {
			return nil, err
		}

		if target != "." {
			if len(envToFill) == 0 {
				return nil, fmt.Errorf("package %q not found", pkgName)
			}
			env[target] = reflectlang.LowerStruct(env, envToFill)
		}

		return nil, nil
	})

	return env
}
