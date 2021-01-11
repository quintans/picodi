package picodi

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

const (
	wireTagKey        = "wire"
	wireFlagTransient = "transient"
)

// Named defines the type for the key for the map that groups all the same types, distinguished by name
type Named string

var (
	namedType = reflect.TypeOf(Named(""))
)

type NamedProviders map[string]interface{}

// AfterWirer is an interface for any implementation that wants to something after being wired.
type AfterWirer interface {
	AfterWire() error
}

type providerFunc func() (interface{}, error)

type injector struct {
	provider  providerFunc
	instance  interface{}
	transient bool
	typ       reflect.Type
}

// PicoDI is a tiny framework for Dependency Injection.
type PicoDI struct {
	namedInjectors map[string]*injector
	typeInjectors  map[reflect.Type]*injector
}

// New creates a new PicoDI instance
func New() *PicoDI {
	return &PicoDI{
		namedInjectors: map[string]*injector{},
		typeInjectors:  map[reflect.Type]*injector{},
	}
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
func (di *PicoDI) NamedProvider(name string, provider interface{}) error {
	return di.namedProvider(name, provider, false)
}

func (di *PicoDI) NamedProviders(providers NamedProviders) error {
	for k, v := range providers {
		if k == "" {
			return errors.New("name cannot be empty")
		}
		err := di.namedProvider(k, v, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) NamedTransientProviders(providers NamedProviders) error {
	for k, v := range providers {
		if k == "" {
			return errors.New("name cannot be empty")
		}
		err := di.namedProvider(k, v, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) Providers(providers ...interface{}) error {
	for _, v := range providers {
		err := di.namedProvider("", v, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) TransientProviders(providers ...interface{}) error {
	for _, v := range providers {
		err := di.namedProvider("", v, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) NamedTransientProvider(name string, provider interface{}) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}
	return di.namedProvider(name, provider, true)
}

func (di *PicoDI) namedProvider(name string, provider interface{}, transient bool) error {
	v := reflect.ValueOf(provider)
	t := v.Type()
	var tn reflect.Type
	var fn providerFunc
	if v.Kind() == reflect.Func {
		// validate function format. It should be `func(...any) any` or `func(...any) (any, error)`
		validateProviderFunc(t)

		fn = func() (interface{}, error) {
			return di.funcInjection(v)
		}
		tn = t.Out(0)
	} else {
		fn = func() (interface{}, error) {
			return provider, nil
		}
		tn = t
	}

	inj := &injector{fn, nil, transient, tn}

	if name != "" {
		// name must be already registered
		v, ok := di.namedInjectors[name]
		if ok {
			return fmt.Errorf("name already registered for type %s", v.typ)
		}
		di.namedInjectors[name] = inj
	} else {
		_, ok := di.typeInjectors[tn]
		if ok {
			return fmt.Errorf("type already registered: %s", tn)
		}
		di.typeInjectors[tn] = inj
	}

	return nil
}

func validateProviderFunc(t reflect.Type) error {
	// must return 1 or 2 results
	if t.NumOut() < 1 && t.NumOut() > 2 {
		return fmt.Errorf("invalid provider function '%s'. Must return at least 1 value. Optionally can also return an error", t.Name())
	}
	// if we have 2 outputs, the second result must be an error
	if t.NumOut() == 2 {
		_, ok := t.Out(1).(error)
		if !ok {
			return fmt.Errorf("invalid provider function '%s'. Second return value must be an error", t.Name())
		}
	}

	return nil
}

func (di *PicoDI) funcInjection(provider reflect.Value) (interface{}, error) {
	t := provider.Type()
	argc := t.NumIn()
	argv := make([]reflect.Value, argc)
	for i := 0; i < argc; i++ {
		at := t.In(i)
		if at.Kind() == reflect.Map && at.Key() == namedType {
			valueType := at.Elem()
			// create map
			var aMapType = reflect.MapOf(namedType, valueType)
			aMap := reflect.MakeMapWithSize(aMapType, 0)
			// find all named type
			for name, inj := range di.namedInjectors {
				// implements an interface or it is of same type
				if valueType.Kind() == reflect.Interface && inj.typ.Implements(valueType) || inj.typ == valueType {
					v, err := di.getByName(name, false)
					if err != nil {
						return nil, err
					}

					aMap.SetMapIndex(reflect.ValueOf(Named(name)), reflect.ValueOf(v))
				}
			}
			argv[i] = aMap
		} else {
			arg, err := di.getByType(at, false)
			if err != nil {
				return nil, err
			}
			argv[i] = reflect.ValueOf(arg)
		}
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
	return di.getByType(t, false)
}

// Resolve returns the instance by name
func (di *PicoDI) Resolve(name string) (interface{}, error) {
	return di.getByName(name, false)
}

func (di *PicoDI) getByName(name string, transient bool) (interface{}, error) {
	inj, ok := di.namedInjectors[name]
	if !ok {
		return nil, fmt.Errorf("no provider was found for name %s", name)
	}

	return di.get(inj, transient)
}

func (di *PicoDI) getByType(t reflect.Type, transient bool) (interface{}, error) {
	if t.Kind() == reflect.Interface {
		for _, v := range di.typeInjectors {
			if v.typ.Implements(t) {
				return di.get(v, transient)
			}
		}
		return nil, fmt.Errorf("no implementation was found for interface type %s", t)
	}

	inj, ok := di.typeInjectors[t]
	if !ok {
		return nil, fmt.Errorf("no provider was found for type %s", t)
	}

	return di.get(inj, transient)
}

func (di *PicoDI) get(inj *injector, transient bool) (interface{}, error) {
	if inj.transient || transient {
		return di.instantiateAndWire(inj)
	}

	if inj.instance == nil {
		provider, err := di.instantiateAndWire(inj)
		if err != nil {
			return nil, err
		}
		inj.instance = provider
	}

	return inj.instance, nil
}

func (di *PicoDI) instantiateAndWire(inj *injector) (interface{}, error) {
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
	if err := di.wireFields(val); err != nil {
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
		return fmt.Errorf("the wiring must be an interface, pointer or  'func (...any) [error]': %#v", value)
	}

	if t == reflect.Func {
		err := validateWireFunc(val.Type())
		if err != nil {
			return err
		}
		_, err = di.funcInjection(val)
		return err
	}

	return di.wireFields(val)
}

func validateWireFunc(t reflect.Type) error {
	// must have 1 or more arguments
	if t.NumIn() == 0 {
		return fmt.Errorf("invalid wire function '%s'. Must have 1 or more inputs", t.Name())
	}
	// must return 1 or 2 results
	if t.NumOut() > 1 {
		return fmt.Errorf("invalid wire function '%s'. It should have no return or only return error", t.Name())
	}
	// if return exists, it must be an error
	if t.NumOut() == 1 {
		_, ok := t.Out(0).(error)
		if !ok {
			return fmt.Errorf("invalid wire function '%s'. Return value must be an error", t.Name())
		}
	}

	return nil
}

func (di *PicoDI) wireFields(val reflect.Value) error {
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
			transient := false
			for _, v := range splits {
				if v == wireFlagTransient {
					transient = true
				}
			}

			var v interface{}
			var err error
			name = splits[0]
			if name == "" {
				v, err = di.getByType(f.Type, transient)
			} else {
				v, err = di.getByName(name, transient)
			}

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
