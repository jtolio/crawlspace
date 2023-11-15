package reflectlang

import (
	"reflect"
	"unsafe"
)

func convert(v reflect.Value, t reflect.Type) reflect.Value {
	switch t {
	case reflect.TypeOf(unsafe.Pointer(nil)):
		switch v.Type() {
		case reflect.TypeOf(uintptr(0)):
			return reflect.ValueOf(unsafe.Pointer(v.Interface().(uintptr)))
		default:
		}
	case reflect.TypeOf(uintptr(0)):
		switch v.Type() {
		case reflect.TypeOf(unsafe.Pointer(nil)):
			return reflect.ValueOf(uintptr(v.UnsafePointer()))
		default:
		}
	default:
	}
	return v.Convert(t)
}
