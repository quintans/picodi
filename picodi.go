package picodi

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

const (
	wireTagKey = "wire"
)

var (
	ErrProviderNotFound       = errors.New("no provider was found")
	ErrProviderAlreadyExists  = errors.New("provider already exists")
	ErrMultipleProvidersFound = errors.New("multiple providers were found")
)

// Named defines the type for the key for the map that groups all the same types, distinguished by name
type Named string

var (
	namedType = reflect.TypeOf(Named(""))
	errorType = reflect.TypeOf((*error)(nil)).Elem()
	cleanType = reflect.TypeOf((*Clean)(nil)).Elem()
)

type NamedProviders map[string]any

// AfterWirer is an interface for any implementation that wants to something after being wired.
type AfterWirer interface {
	AfterWire() (Clean, error)
}

type providerFunc func(dryRun bool) (any, Clean, error)
type Clean func()

type injector struct {
	provider  providerFunc
	instance  any
	clean     Clean
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

func GetByType[T any](di *PicoDI) (T, error) {
	var zero T
	t := reflect.TypeOf(zero)
	a, _, err := di.getByType(t, false)
	if err != nil {
		return zero, err
	}
	at, ok := a.(T)
	if !ok {
		return zero, fmt.Errorf("expected %T, got %T", new(T), a)
	}

	return at, nil
}

// Resolve returns the instance by name
func Resolve[T any](di *PicoDI, name string) (T, error) {
	var zero T
	a, _, err := di.getByName(name, false)
	if err != nil {
		return zero, err
	}
	at, ok := a.(T)
	if !ok {
		return zero, fmt.Errorf("expected %T, got %T", new(T), a)
	}

	return at, nil
}

// NamedProvider register a provider.
//
//	This is used like:
//
//	type Foo struct { Bar string }
//	PicoDI.NamedProvider("foo", func () Foo {
//		return Foo{}
//	})
//
// or
//
//	PicoDI.NamedProvider("foo", Foo{})
//
// In both cases an entry is also created for the full type name. eg: `github.com/quintans/picodi/Foo`
// Registering with an empty name will only register with the full type name.
// If the returned value of the provider is to be wired, it must return a pointer or interface
func (di *PicoDI) NamedProvider(name string, provider any) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}
	return di.namedProvider(name, provider, false)
}

func (di *PicoDI) NamedProviders(providers NamedProviders) error {
	for k, v := range providers {
		err := di.NamedProvider(k, v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) NamedTransientProviders(providers NamedProviders) error {
	for k, v := range providers {
		err := di.NamedTransientProvider(k, v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) Providers(providers ...any) error {
	for _, v := range providers {
		err := di.namedProvider("", v, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) TransientProviders(providers ...any) error {
	for _, v := range providers {
		err := di.namedProvider("", v, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (di *PicoDI) NamedTransientProvider(name string, provider any) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}
	return di.namedProvider(name, provider, true)
}

// GetByType returns the instance by Type
func (di *PicoDI) GetByType(zero any) (any, error) {
	t := reflect.TypeOf(zero)
	instance, _, err := di.getByType(t, false)
	return instance, err
}

// Resolve returns the instance by name
func (di *PicoDI) Resolve(name string) (any, error) {
	instance, _, err := di.getByName(name, false)
	return instance, err
}

// Wire injects dependencies into the instance.
// Dependencies marked for wiring without name will be mapped to their type name.
// After wiring, if the passed value respects the "AfterWirer" interface, "AfterWire() error" will be called
// A clean function is also returned to do any cleaning, like database disconnecting
func (di *PicoDI) Wire(value any) error {
	return di.wire(value, false)
}

// DryRun checks if existing wiring is possible.
// It is the same as Wire() but without instantiating anything.
// This method should be used in unit testing to check if the wiring is correct.
// This way we avoid to boot the whole application just to check if we made some mistake.
func (di *PicoDI) DryRun(value any) error {
	return di.wire(value, true)
}

// Destroy calls all the registered clean functions and reset the PicoDI instance.
func (di *PicoDI) Destroy() {
	for _, inj := range di.namedInjectors {
		if inj.clean != nil {
			inj.clean()
			inj.clean = nil
			inj.instance = nil
		}
	}
	di.namedInjectors = map[string]*injector{}

	for _, inj := range di.typeInjectors {
		if inj.clean != nil {
			inj.clean()
			inj.clean = nil
			inj.instance = nil
		}
	}
	di.typeInjectors = map[reflect.Type]*injector{}
}

func (di *PicoDI) namedProvider(name string, provider any, transient bool) error {
	v := reflect.ValueOf(provider)
	t := v.Type()
	var tn reflect.Type
	var fn providerFunc
	if v.Kind() == reflect.Func {
		// validate function format. It should be `func(...any) any` or `func(...any) (any, error)`
		err := validateProviderFunc(t)
		if err != nil {
			return err
		}

		fn = func(dryRun bool) (any, Clean, error) {
			return di.funcInjection(v, dryRun)
		}
		tn = t.Out(0)
	} else {
		fn = func(_ bool) (any, Clean, error) {
			return provider, nil, nil
		}
		tn = t
	}

	inj := &injector{fn, nil, nil, transient, tn}

	if name != "" {
		// name must be already registered
		v, ok := di.namedInjectors[name]
		if ok {
			return fmt.Errorf("name already registered for type %s: %w", v.typ, ErrProviderAlreadyExists)
		}
		di.namedInjectors[name] = inj
	} else {
		_, ok := di.typeInjectors[tn]
		if ok {
			return fmt.Errorf("type already registered: %s: %w", tn, ErrProviderAlreadyExists)
		}
		di.typeInjectors[tn] = inj
	}

	return nil
}

func validateProviderFunc(t reflect.Type) error {
	// must return 1, 2 or 3 results
	if t.NumOut() < 1 && t.NumOut() > 3 {
		return fmt.Errorf("invalid provider function '%s'. Must return at least 1 value. Optionally can also return a clean function and/or error", t)
	}
	// if we have 3 outputs, the last result must be an error
	if t.NumOut() == 3 {
		if !t.Out(2).AssignableTo(errorType) {
			return fmt.Errorf("invalid provider function '%s'. Third return value must be an error", t)
		}
		if !t.Out(1).AssignableTo(cleanType) {
			return fmt.Errorf("invalid provider function '%s'. Second return value must be '%s'", t, cleanType)
		}
		return nil
	}
	if t.NumOut() == 2 {
		o := t.Out(1)
		if !o.AssignableTo(cleanType) && !o.AssignableTo(errorType) {
			return fmt.Errorf("invalid provider function '%s'. Second return value must be an error or '%s'", t, cleanType)
		}
		return nil
	}

	return nil
}

func (di *PicoDI) funcInjection(provider reflect.Value, dryRun bool) (v any, c Clean, err error) {
	t := provider.Type()
	argc := t.NumIn()
	argv := make([]reflect.Value, argc)
	var cleans []Clean
	cleanDeps := func() {
		for _, v := range cleans {
			v()
		}
		cleans = nil
	}

	defer func() {
		if err != nil {
			cleanDeps()
		}
	}()
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
					v, clean, err := di.getByName(name, dryRun)
					if err != nil {
						return nil, nil, err
					}
					if clean != nil {
						cleans = append(cleans, clean)
					}

					aMap.SetMapIndex(reflect.ValueOf(Named(name)), reflect.ValueOf(v))
				}
			}
			if aMap.Len() == 0 {
				return nil, nil, fmt.Errorf("%w: for named type %s", ErrProviderNotFound, t)
			}

			argv[i] = aMap
		} else {
			arg, clean, err := di.getByType(at, dryRun)
			if err != nil {
				return nil, nil, err
			}
			if clean != nil {
				cleans = append(cleans, clean)
			}
			argv[i] = reflect.ValueOf(arg)
		}
	}

	if dryRun {
		if t.NumOut() == 0 {
			return nil, nil, nil
		}
		return reflect.Zero(t.Out(0)).Interface(), nil, nil
	}

	results := provider.Call(argv)

	var clean Clean
	clear := func() {
		cleanDeps()
		if clean != nil {
			clean()
		}
	}

	// first wiring function
	if len(results) == 0 {
		return nil, clear, nil
	}

	var value any

	if len(results) > 0 {
		value = results[0].Interface()
	}

	if len(results) > 1 {
		e := results[len(results)-1].Interface()
		if er, ok := e.(error); ok {
			return nil, nil, er
		}
		e = results[1].Interface()
		if c, ok := e.(Clean); ok {
			clean = c
		}
	}

	return value, clear, err
}

func (di *PicoDI) getByName(name string, dryRun bool) (any, Clean, error) {
	inj, ok := di.namedInjectors[name]
	if !ok {
		return nil, nil, fmt.Errorf("%w: for name '%s'", ErrProviderNotFound, name)
	}

	return di.get(inj, dryRun)
}

func (di *PicoDI) getByType(t reflect.Type, dryRun bool) (any, Clean, error) {
	if t.Kind() == reflect.Interface {
		// collects all the instances that respect the interface
		matches := []*injector{}
		for _, v := range di.typeInjectors {
			if v.typ.Implements(t) {
				matches = append(matches, v)
			}
		}
		if len(matches) == 1 {
			return di.get(matches[0], dryRun)
		}
		if len(matches) > 1 {
			return nil, nil, fmt.Errorf("%w: for interface type %s", ErrMultipleProvidersFound, t)
		}
		return nil, nil, fmt.Errorf("%w: for interface type %s", ErrProviderNotFound, t)
	}

	inj, ok := di.typeInjectors[t]
	if !ok {
		return nil, nil, fmt.Errorf("%w: for type %s", ErrProviderNotFound, t)
	}

	return di.get(inj, dryRun)
}

func (di *PicoDI) get(inj *injector, dryRun bool) (any, Clean, error) {
	if inj.transient || dryRun {
		return di.instantiateAndWire(inj, dryRun)
	}

	if inj.instance == nil {
		provider, clean, err := di.instantiateAndWire(inj, dryRun)
		if err != nil {
			return nil, nil, err
		}
		inj.instance = provider
		if clean != nil {
			inj.clean = func() {
				if clean != nil {
					clean()
					clean = nil
				}
				inj.instance = nil
				inj.clean = nil
			}
		}
	}

	return inj.instance, inj.clean, nil
}

func (di *PicoDI) instantiateAndWire(inj *injector, dryRun bool) (any, Clean, error) {
	v, clean1, err := inj.provider(dryRun)
	if err != nil {
		return nil, nil, err
	}
	var clean2 Clean
	val := reflect.ValueOf(v)
	k := val.Kind()
	switch k {
	case reflect.Interface, reflect.Pointer:
		k = val.Elem().Kind()
	case reflect.Struct:
		ptr := reflect.New(reflect.TypeOf(v))
		ptr.Elem().Set(val)
		val = ptr
	}

	if k == reflect.Struct {
		clean2, err = di.wireFields(val, dryRun)
		if err != nil {
			return nil, nil, err
		}
	}

	c := func() {
		if clean1 != nil {
			clean1()
			clean1 = nil
		}
		if clean2 != nil {
			clean2()
			clean2 = nil
		}
	}

	return v, c, nil
}

func (di *PicoDI) wire(value any, dryRun bool) error {
	val := reflect.ValueOf(value)
	t := val.Kind()
	if t != reflect.Interface && t != reflect.Pointer && t != reflect.Func {
		// the first wiring must be valid
		return fmt.Errorf("the wiring must be an 'interface', 'pointer' or 'func (...any) [error]': %#v", value)
	}

	if t == reflect.Func {
		err := validateWireFunc(val.Type())
		if err != nil {
			return err
		}
		_, _, err = di.funcInjection(val, dryRun)
		return err
	}

	_, err := di.wireFields(val, dryRun)
	return err
}

func validateWireFunc(t reflect.Type) error {
	// must have 1 or more arguments
	if t.NumIn() == 0 {
		return fmt.Errorf("invalid wire function '%s'. Must have 1 or more inputs", t)
	}
	// must return 1 or 2 results
	if t.NumOut() > 1 {
		return fmt.Errorf("invalid wire function '%s'. It should have no return or only return error", t)
	}
	// if return exists, it must be an error
	if t.NumOut() == 1 {
		_, ok := t.Out(0).(error)
		if !ok {
			return fmt.Errorf("invalid wire function '%s'. Return value must be an error", t)
		}
	}

	return nil
}

func (di *PicoDI) wireFields(val reflect.Value, dryRun bool) (c Clean, err error) {
	k := val.Kind()
	if k != reflect.Pointer && k != reflect.Interface {
		return nil, nil
	}
	// gets the inner struct
	s := val.Elem()
	t := s.Type()
	if t.Kind() != reflect.Struct {
		return nil, nil
	}

	var cleans []Clean
	cleanDeps := func() {
		for _, v := range cleans {
			v()
		}
		cleans = nil
	}

	defer func() {
		if err != nil {
			cleanDeps()
		}
	}()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		if name, ok := f.Tag.Lookup(wireTagKey); ok {
			var v any
			var err error
			var clean Clean
			if name == "" {
				v, clean, err = di.getByType(f.Type, dryRun)
			} else {
				v, clean, err = di.getByName(name, dryRun)
			}
			if err != nil {
				return nil, err
			}

			if clean != nil {
				cleans = append(cleans, clean)
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
		clean, err := aw.AfterWire()
		c := func() {
			cleanDeps()
			if clean != nil {
				clean()
				clean = nil
			}
		}
		return c, err
	}

	return cleanDeps, nil
}
