package picodi

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

// Wire is an interface for any implementation that has to implement wiring.
type Wire interface {
	Provider(string, interface{})
}
type providerFunc func() (interface{}, error)

type injector struct {
	provider providerFunc
	instance interface{}
}

// PicoDI is a tiny framework for Dependency Injection.
type PicoDI struct {
	injectors map[string]injector
}

var _ Wire = &PicoDI{}

type chain struct {
	field string
	name  string
}

func (c chain) String() string {
	return c.field + " \"" + c.name + "\""
}

// New creates a new PicoDI instance
func New() *PicoDI {
	return &PicoDI{
		injectors: make(map[string]injector),
	}
}

// Provider register a provider.
//	This is used like:
//
//	type Foo struct { Bar string }
//	PicoDI.Register("foo", func () Foo {
//		return Foo{}
//	})
//
// or
//
//	PicoDI.Register("foo", Foo{})
//
// In both cases an entry is also created for the full tpye name. eg: `github.com/quintans/picodi/Foo`
// Registering with an empty name will only register with the full type name.
//	If the returned value of the provider is to be wired, it must return a pointer or interface
func (pdi *PicoDI) Provider(name string, provider interface{}) {
	v := reflect.ValueOf(provider)
	var tn string
	var fn providerFunc
	if v.Kind() == reflect.Func {
		fn = func() (interface{}, error) {
			return pdi.constructorInjection(v)
		}
		tn = typeName(v.Type().Out(0))
	} else {
		fn = func() (interface{}, error) { return provider, nil }
		tn = typeName(v.Type())
	}

	inj := injector{fn, nil}

	if name != "" {
		pdi.injectors[name] = inj
	}
	if tn != name {
		pdi.injectors[tn] = inj
	}
}

func typeName(t reflect.Type) string {
	var star string
	k := t.Kind()
	if k == reflect.Ptr {
		star = "*"
		t = t.Elem()
	}
	return star + t.PkgPath() + "/" + t.Name()
}

func (pdi *PicoDI) constructorInjection(provider reflect.Value) (interface{}, error) {
	t := provider.Type()
	argc := t.NumIn()
	argv := make([]reflect.Value, argc)
	for i := 0; i < argc; i++ {
		at := t.In(i)
		v, err := pdi.Resolve(typeName(at))
		if err != nil {
			return nil, err
		}
		argv[i] = reflect.ValueOf(v)
	}
	result := provider.Call(argv)
	return result[0].Interface(), nil
}

// GetByType returns the instance by Type
func (pdi *PicoDI) GetByType(zero interface{}) (interface{}, error) {
	t := reflect.TypeOf(zero)
	return pdi.get(make([]chain, 0), typeName(t))
}

// Resolve returns the instance by name
func (pdi *PicoDI) Resolve(name string) (interface{}, error) {
	return pdi.get(make([]chain, 0), name)
}

func (pdi *PicoDI) get(fetching []chain, name string) (interface{}, error) {
	inj, ok := pdi.injectors[name]
	if !ok {
		return nil, fmt.Errorf("No provider was found for %s: %s", name, joinChain(fetching))
	}

	if inj.instance == nil {
		v, err := inj.provider()
		if err != nil {
			return nil, err
		}
		val := reflect.ValueOf(v)
		k := val.Kind()
		if k == reflect.Struct {
			ptr := reflect.New(reflect.TypeOf(v))
			ptr.Elem().Set(val)
			val = ptr
		}
		if err := pdi.wire(fetching, val); err != nil {
			return nil, err
		}
		inj.instance = v
	}

	return inj.instance, nil
}

// Wire injects dependencies into the instance.
// Dependencies marked for wiring without name will be mapped to their type name.
func (pdi *PicoDI) Wire(value interface{}) error {
	val := reflect.ValueOf(value)
	t := val.Kind()
	if t != reflect.Interface && t != reflect.Ptr {
		// the first wiring must be valid
		var err = fmt.Sprintf("The first wiring must be a interface or a pointer: %#v", value)
		return errors.New(err)
	}
	return pdi.wire(make([]chain, 0), val)
}

func (pdi *PicoDI) wire(fetching []chain, val reflect.Value) error {
	k := val.Kind()
	if k != reflect.Ptr && k != reflect.Interface {
		return nil
	}
	// gets the inner struct
	s := val.Elem()
	t := s.Type()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		if name, ok := f.Tag.Lookup("wire"); ok {
			if name == "" {
				name = typeName(f.Type)
			}

			var fieldName = t.String() + "." + f.Name
			var link = chain{fieldName, name}
			var names = append(fetching, link)
			var v, err = pdi.get(names, name)
			if err != nil {
				return err
			}

			var fieldValue = s.Field(i)
			if fieldValue.CanSet() {
				fieldValue.Set(reflect.ValueOf(v))
			} else if method := val.MethodByName("Set" + strings.Title(f.Name)); method.IsValid() {
				// Setter defined for the pointer
				method.Call([]reflect.Value{reflect.ValueOf(v)})
			} else {
				// Cheat: writting to unexported fields
				fld := reflect.NewAt(fieldValue.Type(), unsafe.Pointer(fieldValue.UnsafeAddr())).Elem()
				fld.Set(reflect.ValueOf(v))
			}
		}
	}

	return nil
}

func joinChain(fetching []chain) string {
	var s = make([]string, len(fetching))
	for k, v := range fetching {
		s[k] = v.String()
	}

	return strings.Join(s, "->")
}
