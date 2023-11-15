package reflectlang

import (
	"fmt"
	"reflect"
)

type Environment map[string]reflect.Value

func NewStandardEnvironment() Environment {
	env := Environment{}
	env["nil"] = reflect.ValueOf(nil)
	env["true"] = reflect.ValueOf(true)
	env["false"] = reflect.ValueOf(false)
	env["$import"] = LowerFunc(env, func(args []reflect.Value) ([]reflect.Value, error) {
		return nil, fmt.Errorf("import unsupported in this session")
	})

	assignment := func(mutate bool) reflect.Value {
		return LowerFunc(env, func(lhs []reflect.Value) ([]reflect.Value, error) {
			for _, arg := range lhs {
				if arg.Kind() != reflect.String {
					return nil, fmt.Errorf("programmer error")
				}
				key := arg.String()
				if mutate {
					if _, exists := env[key]; !exists {
						return nil, fmt.Errorf("variable %q does not exist", key)
					}
				} else {
					if _, exists := env[key]; exists {
						return nil, fmt.Errorf("variable %q already exists", key)
					}
				}
			}
			return []reflect.Value{
				LowerFunc(env, func(rhs []reflect.Value) ([]reflect.Value, error) {
					if len(lhs) != len(rhs) {
						return nil, fmt.Errorf("variable definition expected a variable for each value (%d != %d)", len(lhs), len(rhs))
					}
					for i, arg := range lhs {
						env[arg.String()] = rhs[i]
					}
					return []reflect.Value{}, nil
				})}, nil
		})
	}

	env["$define"] = assignment(false)
	env["$mutate"] = assignment(true)

	env["len"] = LowerFunc(env, func(args []reflect.Value) ([]reflect.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("len expected 1 argument")
		}
		return []reflect.Value{reflect.ValueOf(args[0].Len())}, nil
	})

	return env
}
