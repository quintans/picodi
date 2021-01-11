package picodi_test

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"github.com/quintans/picodi"
	"github.com/stretchr/testify/require"
)

type Namer interface {
	Name() string
}

type Foo struct {
	name string
}

func (foo Foo) Name() string {
	return foo.name
}

type Bar struct {
	Foo       Foo   `wire:"foo"`
	Foo2      Foo   `wire:""`
	Other     Namer `wire:"foo"`
	inner     *Foo  `wire:"fooptr"`
	inner2    Foo   `wire:"foo"`
	Fun       Foo   `wire:"foofn"`
	Fun2      Foo   `wire:"foofn,transient"` // a new instance will be created
	FooPtr    *Foo  `wire:"fooptr"`
	afterWire bool
}

func (b *Bar) SetInner(v *Foo) {
	b.inner = v
}

func (b *Bar) AfterWire() error {
	// after wire called
	b.afterWire = true
	return nil
}

func TestStructWire(t *testing.T) {
	rand.Seed(42)
	pico := picodi.New()
	pico.NamedProvider("fooptr", &Foo{"Foo"})
	pico.NamedProvider("foo", Foo{"Foo"})
	pico.NamedProvider("foofn", func() Foo {
		return Foo{fmt.Sprintf("FooFn-%d", rand.Intn(99))}
	})
	pico.NamedProvider("", Foo{"Foo"})

	var bar = Bar{}
	if err := pico.Wire(&bar); err != nil {
		t.Fatal("Unexpected error when wiring bar: ", err)
	}

	require.True(t, bar.afterWire, "AfterWire() was not called")

	if bar.Foo.Name() != "Foo" {
		t.Fatal("Expected \"Foo\" for Foo, got", bar.Foo.Name())
	}

	if bar.FooPtr.Name() != "Foo" {
		t.Fatal("Expected \"Foo\" for FooPtr, got", bar.FooPtr.Name())
	}

	if bar.Other.Name() != "Foo" {
		t.Fatal("Expected \"Foo\" for Other, got", bar.Other.Name())
	}

	if bar.Foo2.Name() != "Foo" {
		t.Fatal("Expected \"Foo\" for Foo2, got", bar.Foo2.Name())
	}

	if bar.inner.Name() != "Foo" {
		t.Fatal("Expected \"Foo\" for inner, got", bar.inner.Name())
	}

	if bar.inner2.Name() != "Foo" {
		t.Fatal("Expected \"Foo\" for inner2, got", bar.inner.Name())
	}

	if bar.Fun.Name() != "FooFn-44" {
		t.Fatal("Expected \"FooFn\" for Fun, got", bar.Fun.Name())
	}

	require.Equal(t, &bar.Foo, &bar.Foo2, "Injected instances are not singletons")
	// Fun2, marked as transient, will have different instance
	require.NotEqual(t, bar.Fun, bar.Fun2, "Injected instances are not transients")
}

type Faulty struct {
	bar Bar `wire:"missing"`
}

func TestErrorWire(t *testing.T) {
	var pico = picodi.New()
	err := pico.Wire(&Faulty{})
	require.Error(t, err, "Expected error for missing provider, nothing")
}

type Message string

func NewMessage() Message {
	return Message("Hi there!")
}

type Greeter interface {
	Greet() Message
}

type GreeterImpl struct {
	Message Message
	Chaos   int
}

func (g GreeterImpl) Greet() Message {
	return g.Message
}

// NewGreeter returns an implementation of Greeter
func NewGreeter(m Message) *GreeterImpl {
	return &GreeterImpl{
		Message: m,
		Chaos:   rand.Intn(100),
	}
}

type Event struct {
	Greeter Greeter
}

// NewEvent receives a Greeter interface
func NewEvent(g Greeter) Event {
	return Event{Greeter: g}
}

func (e Event) Start() string {
	msg := e.Greeter.Greet()
	return string(msg)
}

func TestWireByName(t *testing.T) {
	var di = picodi.New()
	di.NamedProvider("event", NewEvent)
	di.Providers(NewMessage, NewGreeter)

	e, err := di.Resolve("event")
	require.NoError(t, err)
	event1 := e.(Event)
	actual := event1.Start()
	require.Equal(t, "Hi there!", actual)

	// second resolve should return the same instance
	e, err = di.Resolve("event")
	require.NoError(t, err)
	event2 := e.(Event)
	require.NoError(t, err)
	require.NotNil(t, event2.Greeter)

	if event1.Greeter != event1.Greeter {
		t.Fatal("Injected instances are not singletons")
	}
}

func TestWireFuncByName(t *testing.T) {
	di := picodi.New()
	di.NamedProviders(picodi.NamedProviders{
		"message1": "hello",
		"message2": "world",
		"message3": 1, // this will not inject
	})

	// only strings will passed to factory
	err := di.Wire(func(m map[picodi.Named]string) {
		require.Equal(t, m["message1"], "hello")
		require.Equal(t, m["message2"], "world")
		require.Len(t, m, 2)
	})
	require.NoError(t, err)
}

func TestWire(t *testing.T) {
	var di = picodi.New()
	di.NamedProviders(picodi.NamedProviders{
		"event": NewEvent,
	})
	di.Providers(NewMessage, NewGreeter)

	e, err := di.Resolve("event")
	require.NoError(t, err)
	event1a := e.(Event)
	actual := event1a.Start()
	require.Equal(t, "Hi there!", actual)

	e, err = di.Resolve("event")
	require.NoError(t, err)
	event1b := e.(Event)

	if event1a.Greeter != event1b.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	di.Providers(NewEvent)
	e, err = di.GetByType(Event{})
	require.NoError(t, err)
	event2a := e.(Event)
	require.NotNil(t, event2a.Greeter)

	e, err = di.GetByType(Event{})
	require.NoError(t, err)
	event2b := e.(Event)
	require.NotNil(t, event2b.Greeter)

	if event2a.Greeter != event2b.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	event3a := Event{}
	err = di.Wire(func(g Greeter) {
		event3a.Greeter = g
	})
	require.NoError(t, err)
	require.NotNil(t, event3a.Greeter)

	event3b := Event{}
	err = di.Wire(func(g Greeter) {
		event3b.Greeter = g
	})
	require.NoError(t, err)
	require.NotNil(t, event3b.Greeter)

	if event3a.Greeter != event3b.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	if event1a.Greeter != event2a.Greeter {
		t.Fatal("Injected instances are not singletons")
	}
	if event2a.Greeter != event3a.Greeter {
		t.Fatal("Injected instances are not singletons")
	}
}

func NewGrumpyEvent(g Greeter) (Event, error) {
	return Event{}, errors.New("could not create event: I am grumpy")
}

func TestWireWithError(t *testing.T) {
	var di = picodi.New()
	di.NamedProviders(picodi.NamedProviders{
		"event":   NewGrumpyEvent,
		"message": NewMessage,
		"greeter": NewGreeter,
	})

	_, err := di.Resolve("event") // will wire if not already
	require.Error(t, err)
}
