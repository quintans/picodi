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

func (b *Bar) AfterWire() (picodi.Clean, error) {
	// after wire called
	b.afterWire = true
	return nil, nil
}

func TestStructWire(t *testing.T) {
	counter := 0
	di := picodi.New()
	di.NamedProvider("fooptr", &Foo{"Foo"})
	di.NamedProvider("foo", Foo{"Foo"})
	di.NamedProvider("foofn", func() Foo {
		counter++
		return Foo{fmt.Sprintf("FooFn-%d", counter)}
	})
	di.Providers(Foo{"Foo"})

	var bar = Bar{}
	_, err := di.DryRun(&bar)
	require.NoError(t, err)

	_, err = di.Wire(&bar)
	require.NoError(t, err)

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

	if bar.Fun.Name() != "FooFn-1" {
		t.Fatal("Expected \"FooFn-1\" for Fun, got", bar.Fun.Name())
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
	_, err := pico.Wire(&Faulty{})
	require.Contains(t, err.Error(), "no provider was found for name")
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
func NewGreeter(m Message) (*GreeterImpl, picodi.Clean, error) {
	g := &GreeterImpl{
		Message: m,
		Chaos:   rand.Intn(100),
	}
	return g, func() {
		if g.Chaos <= -1 {
			// should never be called
			g.Chaos--
		} else {
			g.Chaos = -1
		}
	}, nil
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
	err := di.NamedProvider("event", NewEvent)
	require.NoError(t, err)
	err = di.Providers(NewMessage, NewGreeter)
	require.NoError(t, err)

	var clean1 picodi.Clean
	e, clean1, err := di.Resolve("event")
	require.NoError(t, err)
	event1 := e.(Event)
	actual := event1.Start()
	require.Equal(t, "Hi there!", actual)

	// second resolve should return the same instance
	var clean2 picodi.Clean
	e, clean2, err = di.Resolve("event")
	require.NoError(t, err)
	event2 := e.(Event)
	require.NoError(t, err)
	require.NotNil(t, event2.Greeter)

	if event1.Greeter != event1.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	g := event1.Greeter.(*GreeterImpl)
	require.NotEqual(t, -1, g.Chaos)
	clean1()
	require.Equal(t, -1, g.Chaos)
	// clean1 == clean2 and calling a second time will have no effect
	clean2()
	require.Equal(t, -1, g.Chaos)
}

func TestWireFuncByName(t *testing.T) {
	di := picodi.New()
	di.NamedProviders(picodi.NamedProviders{
		"message1": "hello",
		"message2": "world",
		"message3": 1, // this will not inject
	})

	var m1, m2 string
	size := 0

	fn := func(m map[picodi.Named]string) {
		m1 = m["message1"]
		m2 = m["message2"]
		size = len(m)
	}

	_, err := di.DryRun(fn)
	require.NoError(t, err)

	// only strings will passed to factory
	clean, err := di.Wire(fn)
	require.NoError(t, err)
	require.Nil(t, clean)

	require.Equal(t, m1, "hello")
	require.Equal(t, m2, "world")
	require.Equal(t, size, 2)

}

func TestWire(t *testing.T) {
	var di = picodi.New()
	di.NamedProviders(picodi.NamedProviders{
		"event": NewEvent,
	})
	di.Providers(NewMessage, NewGreeter)

	e, _, err := di.Resolve("event")
	require.NoError(t, err)
	event1a := e.(Event)
	actual := event1a.Start()
	require.Equal(t, "Hi there!", actual)

	e, _, err = di.Resolve("event")
	require.NoError(t, err)
	event1b := e.(Event)

	if event1a.Greeter != event1b.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	di.Providers(NewEvent)
	e, _, err = di.GetByType(Event{})
	require.NoError(t, err)
	event2a := e.(Event)
	require.NotNil(t, event2a.Greeter)

	e, _, err = di.GetByType(Event{})
	require.NoError(t, err)
	event2b := e.(Event)
	require.NotNil(t, event2b.Greeter)

	if event2a.Greeter != event2b.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	event3a := Event{}
	_, err = di.Wire(func(g Greeter) {
		event3a.Greeter = g
	})
	require.NoError(t, err)
	require.NotNil(t, event3a.Greeter)

	event3b := Event{}
	_, err = di.Wire(func(g Greeter) {
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

func TestDryRunWithError(t *testing.T) {
	var di = picodi.New()
	err := di.Providers(NewGreeter, NewEvent)
	require.NoError(t, err)

	_, err = di.DryRun(func(event Event) {})
	require.Contains(t, err.Error(), "no provider was found for type")
}

var errGrumpy = errors.New("could not create event: I am grumpy")

func NewGrumpyEvent(g Greeter) (Event, error) {
	return Event{}, errGrumpy
}

func TestWireWithError(t *testing.T) {
	var di = picodi.New()
	err := di.NamedProvider("event", NewGrumpyEvent)
	require.NoError(t, err)
	err = di.Providers(NewMessage, NewGreeter)
	require.NoError(t, err)

	_, _, err = di.Resolve("event") // will wire if not already
	require.Error(t, err)
	require.True(t, errors.Is(err, errGrumpy), err)
}
