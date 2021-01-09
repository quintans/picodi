package picodi

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

// AfterWirer is an interface for any implementation that wants to something after being wired.
type AfterWirer interface {
	AfterWire() error
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
// If the returned value of the provider is to be wired, it must return a pointer or interface
func (di *PicoDI) Provider(name string, provider interface{}) {
	v := reflect.ValueOf(provider)
	t := v.Type()
	var tn string
	var fn providerFunc
	if v.Kind() == reflect.Func {
		// validate function format. It should be `func(...any) any` or `func(...any) (any, error)`
		validateProviderFunc(t)

		fn = func() (interface{}, error) {
			return di.constructorInjection(v)
		}
		tn = typeName(t.Out(0))
	} else {
		fn = func() (interface{}, error) { return provider, nil }
		tn = typeName(t)
	}

	inj := injector{fn, nil}

	if name != "" {
		di.injectors[name] = inj
	}
	if tn != name {
		di.injectors[tn] = inj
	}
}

func validateProviderFunc(t reflect.Type) error {
	// must have 1 or more arguments
	if t.NumIn() == 0 {
		return fmt.Errorf("Invalid function provider '%s'. Must have 1 or more inputs", t.Name())
	}
	// must return 1 or 2 results
	if t.NumOut() < 1 && t.NumOut() > 2 {
		return fmt.Errorf("Invalid function provider '%s'. Must return at least 1 value. Optionally can also return an error", t.Name())
	}
	// if exists, the second result must be an error
	if t.NumIn() == 2 {
		_, ok := t.Out(1).(error)
		if !ok {
			return fmt.Errorf("Invalid function provider '%s'. Second return value must be an error", t.Name())
		}
	}

	return nil
}

func (di *PicoDI) Providers(providers map[string]interface{}) {
	for k, v := range providers {
		di.Provider(k, v)
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

func (di *PicoDI) constructorInjection(provider reflect.Value) (interface{}, error) {
	t := provider.Type()
	argc := t.NumIn()
	argv := make([]reflect.Value, argc)
	for i := 0; i < argc; i++ {
		at := t.In(i)
		v, err := di.Resolve(typeName(at))
		if err != nil {
			return nil, err
		}
		argv[i] = reflect.ValueOf(v)
	}
	result := provider.Call(argv)
	if len(result) == 2 {
		return nil, result[1].Interface().(error)
	}

	return result[0].Interface(), nil
}

// GetByType returns the instance by Type
func (di *PicoDI) GetByType(zero interface{}) (interface{}, error) {
	t := reflect.TypeOf(zero)
	return di.get(make([]chain, 0), typeName(t))
}

// Resolve returns the instance by name
func (di *PicoDI) Resolve(name string) (interface{}, error) {
	return di.get(make([]chain, 0), name)
}

func (di *PicoDI) get(fetching []chain, name string) (interface{}, error) {
	inj, ok := di.injectors[name]
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
		if err := di.wire(fetching, val); err != nil {
			return nil, err
		}
		inj.instance = v
	}

	return inj.instance, nil
}

// Wire injects dependencies into the instance.
// Dependencies marked for wiring without name will be mapped to their type name.
// After wiring, if the passed value respects the "AfterWirer" interface, "AfterWire() error" will be called
func (di *PicoDI) Wire(value interface{}) error {
	val := reflect.ValueOf(value)
	t := val.Kind()
	if t != reflect.Interface && t != reflect.Ptr {
		// the first wiring must be valid
		return fmt.Errorf("The first wiring must be a interface or a pointer: %#v", value)
	}
	err := di.wire(make([]chain, 0), val)
	if err != nil {
		return err
	}

	return nil
}

func (di *PicoDI) wire(fetching []chain, val reflect.Value) error {
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
			var v, err = di.get(names, name)
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

	if aw, ok := val.Interface().(AfterWirer); ok {
		return aw.AfterWire()
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
