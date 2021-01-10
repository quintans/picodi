package picodi

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

const (
	wireTagKey        = "wire"
	wireFlagTransient = "transient"
)

type NamedProviders map[string]interface{}

type Providers []interface{}

// AfterWirer is an interface for any implementation that wants to something after being wired.
type AfterWirer interface {
	AfterWire() error
}

type providerFunc func() (interface{}, error)

type injector struct {
	provider  providerFunc
	instance  interface{}
	transient bool
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

func (di *PicoDI) Provider(provider interface{}) {
	di.namedProvider("", provider, false)
}

// NamedProvider register a provider.
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
// In both cases an entry is also created for the full type name. eg: `github.com/quintans/picodi/Foo`
// Registering with an empty name will only register with the full type name.
// If the returned value of the provider is to be wired, it must return a pointer or interface
func (di *PicoDI) NamedProvider(name string, provider interface{}) {
	di.namedProvider(name, provider, false)
}

func (di *PicoDI) NamedTransientProvider(name string, provider interface{}) {
	di.namedProvider(name, provider, true)
}

func (di *PicoDI) namedProvider(name string, provider interface{}, transient bool) {
	v := reflect.ValueOf(provider)
	t := v.Type()
	var tn string
	var fn providerFunc
	if v.Kind() == reflect.Func {
		// validate function format. It should be `func(...any) any` or `func(...any) (any, error)`
		validateProviderFunc(t)

		fn = func() (interface{}, error) {
			return di.funcInjection(v)
		}
		tn = typeName(t.Out(0))
	} else {
		fn = func() (interface{}, error) {
			return provider, nil
		}
		tn = typeName(t)
	}

	inj := injector{fn, nil, transient}

	if name != "" {
		di.injectors[name] = inj
	}
	if tn != name {
		di.injectors[tn] = inj
	}
}

func validateProviderFunc(t reflect.Type) error {
	// must return 1 or 2 results
	if t.NumOut() < 1 && t.NumOut() > 2 {
		return fmt.Errorf("Invalid provider function '%s'. Must return at least 1 value. Optionally can also return an error", t.Name())
	}
	// if we have 2 outputs, the second result must be an error
	if t.NumOut() == 2 {
		_, ok := t.Out(1).(error)
		if !ok {
			return fmt.Errorf("Invalid provider function '%s'. Second return value must be an error", t.Name())
		}
	}

	return nil
}

func (di *PicoDI) NamedProviders(providers NamedProviders) {
	for k, v := range providers {
		di.namedProvider(k, v, false)
	}
}

func (di *PicoDI) NamedTransientProviders(providers NamedProviders) {
	for k, v := range providers {
		di.namedProvider(k, v, true)
	}
}

func (di *PicoDI) Providers(providers Providers) {
	for _, v := range providers {
		di.namedProvider("", v, false)
	}
}

func (di *PicoDI) TransientProviders(providers Providers) {
	for _, v := range providers {
		di.namedProvider("", v, true)
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

func (di *PicoDI) funcInjection(provider reflect.Value) (interface{}, error) {
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
	results := provider.Call(argv)
	if len(results) == 1 {
		return results[0].Interface(), nil
	}

	if len(results) == 2 {
		e := results[1].Interface()
		if e != nil {
			return nil, e.(error)
		}
	}

	return nil, nil
}

// GetByType returns the instance by Type
func (di *PicoDI) GetByType(zero interface{}) (interface{}, error) {
	t := reflect.TypeOf(zero)
	return di.get([]chain{}, typeName(t), false)
}

// Resolve returns the instance by name
func (di *PicoDI) Resolve(name string) (interface{}, error) {
	return di.get([]chain{}, name, false)
}

func (di *PicoDI) get(fetching []chain, name string, transient bool) (interface{}, error) {
	inj, ok := di.injectors[name]
	if !ok {
		return nil, fmt.Errorf("No provider was found for %s: %s", name, joinChain(fetching))
	}

	if inj.transient || transient {
		return di.instantiateAndWire(fetching, inj)
	}

	if inj.instance == nil {
		provider, err := di.instantiateAndWire(fetching, inj)
		if err != nil {
			return nil, err
		}
		inj.instance = provider
	}

	return inj.instance, nil
}

func (di *PicoDI) instantiateAndWire(fetching []chain, inj injector) (interface{}, error) {
	provider, err := inj.provider()
	if err != nil {
		return nil, err
	}
	val := reflect.ValueOf(provider)
	k := val.Kind()
	if k == reflect.Struct {
		ptr := reflect.New(reflect.TypeOf(provider))
		ptr.Elem().Set(val)
		val = ptr
	}
	if err := di.wire(fetching, val); err != nil {
		return nil, err
	}

	return provider, nil
}

// Wire injects dependencies into the instance.
// Dependencies marked for wiring without name will be mapped to their type name.
// After wiring, if the passed value respects the "AfterWirer" interface, "AfterWire() error" will be called
func (di *PicoDI) Wire(value interface{}) error {
	val := reflect.ValueOf(value)
	t := val.Kind()
	if t != reflect.Interface && t != reflect.Ptr && t != reflect.Func {
		// the first wiring must be valid
		return fmt.Errorf("The wiring must be an interface, pointer or  'func (...any) [error]': %#v", value)
	}

	if t == reflect.Func {
		err := validateWireFunc(val.Type())
		if err != nil {
			return err
		}
		_, err = di.funcInjection(val)
		return err
	}

	return di.wire([]chain{}, val)
}

func validateWireFunc(t reflect.Type) error {
	// must have 1 or more arguments
	if t.NumIn() == 0 {
		return fmt.Errorf("Invalid wire function '%s'. Must have 1 or more inputs", t.Name())
	}
	// must return 1 or 2 results
	if t.NumOut() > 1 {
		return fmt.Errorf("Invalid wire function '%s'. It should have no return or only return error", t.Name())
	}
	// if return exists, it must be an error
	if t.NumOut() == 1 {
		_, ok := t.Out(0).(error)
		if !ok {
			return fmt.Errorf("Invalid wire function '%s'. Return value must be an error", t.Name())
		}
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

		if name, ok := f.Tag.Lookup(wireTagKey); ok {
			splits := strings.Split(name, ",")
			name = splits[0]
			if name == "" {
				name = typeName(f.Type)
			}
			transient := false
			for _, v := range splits {
				if v == wireFlagTransient {
					transient = true
				}
			}

			var fieldName = t.String() + "." + f.Name
			var link = chain{fieldName, name}
			var names = append(fetching, link)
			var v, err = di.get(names, name, transient)
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
