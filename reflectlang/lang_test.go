package reflectlang

import (
	"fmt"
	"reflect"
	"testing"
)

type TestStruct struct {
	Field1 int
	Field2 string
	calls  int
	err    error
}

func (s *TestStruct) SetField1(v int) {
	s.Field1 = v
}

func (s *TestStruct) SetField2(v string) {
	s.Field2 = v
}

func (s *TestStruct) GetField1() int    { return s.Field1 }
func (s *TestStruct) GetField2() string { return s.Field2 }

func (s *TestStruct) TestCall() (int, error) {
	s.calls += 1
	return s.calls, s.err
}

func singleVal(v []reflect.Value, err error) (reflect.Value, error) {
	if err != nil {
		return reflect.Value{}, err
	}
	if len(v) == 0 {
		return reflect.ValueOf(nil), nil
	}
	if len(v) == 1 {
		return v[0], nil
	}
	return reflect.Value{}, fmt.Errorf("multivalue in single value context")
}

func singleEval(script string, env Environment) (reflect.Value, error) {
	return singleVal(Eval(script, env))
}

func TestLang(t *testing.T) {
	s := &TestStruct{}
	env := Environment{
		"s":   reflect.ValueOf(s),
		"nil": reflect.ValueOf(nil),
		"try": reflect.ValueOf(func(v int, err error) int {
			if err != nil {
				panic(err)
			}
			return v
		}),
	}

	rv, err := singleEval("s.GetField1()", env)
	if err != nil {
		t.Fatal(err)
	}
	if rv.Int() != 0 {
		t.Fatal("unexpected")
	}
	_, err = singleEval("s.SetField1(try(s.TestCall()))", env)
	if err != nil {
		t.Fatal(err)
	}
	if s.Field1 != 1 {
		t.Fatal("unexpected")
	}
	_, err = singleEval("s.SetField1(try(s.TestCall()))", env)
	if err != nil {
		t.Fatal(err)
	}
	if s.Field1 != 2 {
		t.Fatal("unexpected")
	}

	rv, err = singleEval("try(s.TestCall())", env)
	if err != nil {
		t.Fatal(err)
	}
	if !rv.CanInt() {
		t.Fatal("unexpected")
	}
}
