package picodi_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/quintans/picodi"
	"github.com/stretchr/testify/assert"
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
	Fun2      Foo   `wire:"foofn2"` // a new instance will be created every time
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
	counterT := 0
	di := picodi.New()
	err := di.NamedProvider("fooptr", &Foo{"Foo"})
	require.NoError(t, err)
	err = di.NamedProvider("foo", Foo{"Foo"})
	require.NoError(t, err)
	err = di.NamedProvider("foofn", func() Foo {
		counter++
		return Foo{fmt.Sprintf("FooFn-%d", counter)}
	})
	require.NoError(t, err)
	err = di.NamedTransientProvider("foofn2", func() Foo {
		counterT++
		return Foo{fmt.Sprintf("FooFn2-%d", counterT)}
	})
	require.NoError(t, err)
	err = di.Providers(Foo{"Foo"})
	require.NoError(t, err)

	var bar = Bar{}
	err = di.DryRun(&bar)
	require.NoError(t, err)

	err = di.Wire(&bar)
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

	err = di.Wire(&bar)
	require.NoError(t, err)
	// Fun2, marked as transient, will have different instance
	assert.Equal(t, "FooFn-1", bar.Fun.Name(), "Injected instances are not singletons")
	assert.Equal(t, "FooFn2-2", bar.Fun2.Name(), "Injected instances are not transients")
}

type Faulty struct {
	bar *Bar `wire:"missing"`
}

func TestErrorWire(t *testing.T) {
	var pico = picodi.New()
	var faulty Faulty
	err := pico.Wire(&faulty)
	require.ErrorIs(t, err, picodi.ErrProviderNotFound)
	assert.Nil(t, faulty.bar)
}

type Message string

func NewMessage() Message {
	return Message("Hi there!")
}

type Greeter interface {
	Greet() Message
}

type GreeterImpl struct {
	Message  Message
	Shutdown int
}

func (g GreeterImpl) Greet() Message {
	return g.Message
}

// NewGreeter returns an implementation of Greeter
func NewGreeter(m Message) (*GreeterImpl, picodi.Clean, error) {
	g := &GreeterImpl{
		Message: m,
	}
	return g, func() {
		g.Shutdown--
	}, nil
}

type Event struct {
	Greeter  Greeter
	Shutdown int
}

// NewEvent receives a Greeter interface
func NewEvent(g Greeter) (*Event, picodi.Clean) {
	e := &Event{Greeter: g}
	return e, func() {
		e.Shutdown--
	}
}

func (e *Event) Start() string {
	msg := e.Greeter.Greet()
	return string(msg)
}

func TestWireByName(t *testing.T) {
	var di = picodi.New()
	err := di.NamedProvider("event", NewEvent)
	require.NoError(t, err)
	err = di.Providers(NewMessage, NewGreeter)
	require.NoError(t, err)

	e, err := di.Resolve("event")
	require.NoError(t, err)
	event1 := e.(*Event)
	actual := event1.Start()
	require.Equal(t, "Hi there!", actual)

	// second resolve should return the same instance
	// another way to get by name
	event2, err := picodi.Resolve[*Event](di, "event")
	require.NoError(t, err)
	require.NotNil(t, event2.Greeter)

	if event1.Greeter != event2.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	g := event1.Greeter.(*GreeterImpl)
	assert.Equal(t, 0, g.Shutdown)
	assert.Equal(t, 0, event1.Shutdown)

	di.Destroy() // decrease chaos
	require.Equal(t, -1, g.Shutdown)
	assert.Equal(t, -1, event1.Shutdown)
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

	err := di.DryRun(fn)
	require.NoError(t, err)

	// only strings will passed to factory
	err = di.Wire(fn)
	require.NoError(t, err)

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

	e, err := di.Resolve("event")
	require.NoError(t, err)
	event1a := e.(*Event)
	actual := event1a.Start()
	require.Equal(t, "Hi there!", actual)

	e, err = di.Resolve("event")
	require.NoError(t, err)
	event1b := e.(*Event)

	if event1a.Greeter != event1b.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	di.Providers(NewEvent)
	e, err = di.GetByType(&Event{})
	require.NoError(t, err)
	event2a := e.(*Event)
	require.NotNil(t, event2a.Greeter)

	// another way to get by type
	event2b, err := picodi.GetByType[*Event](di) // will wire if not already
	require.NoError(t, err)
	require.NotNil(t, event2b.Greeter)

	if event2a.Greeter != event2b.Greeter {
		t.Fatal("Injected instances are not singletons")
	}

	event3a := &Event{}
	err = di.Wire(func(g Greeter) {
		event3a.Greeter = g
	})
	require.NoError(t, err)
	require.NotNil(t, event3a.Greeter)

	event3b := &Event{}
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

func TestDryRunWithError(t *testing.T) {
	var di = picodi.New()
	err := di.Providers(NewGreeter, NewEvent)
	require.NoError(t, err)

	err = di.DryRun(func(event Event) {})
	require.Contains(t, err.Error(), "no provider was found: for type")
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

	_, err = di.Resolve("event") // will wire if not already
	require.Error(t, err)
	require.True(t, errors.Is(err, errGrumpy), err)
}

type Level1 struct {
	Level2 *Level2 `wire:""`
}

type Level2 struct {
	Level3 *Level3 `wire:""`
}

type Level3 struct {
	Value string
}

func TestDependencyTree(t *testing.T) {
	var di = picodi.New()
	err := di.Providers(&Level3{Value: "Hello"})
	require.NoError(t, err)
	err = di.Providers(&Level2{}) // we don't define Level3, it will be injected
	require.NoError(t, err)

	var l1 Level1
	err = di.Wire(&l1)
	require.NoError(t, err)

	require.NotNil(t, l1.Level2)
	require.NotNil(t, l1.Level2.Level3)
	require.Equal(t, l1.Level2.Level3.Value, "Hello")
}

type Stronger interface {
	Strong(ctx context.Context, msg string) string
}

type StrongerImpl struct{}

func (s StrongerImpl) Strong(_ context.Context, msg string) string {
	return strings.ToUpper(msg)
}

type Shout func(context.Context, string) string

func NewShoutHandler(s Stronger) Shout {
	return func(ctx context.Context, msg string) string {
		return fmt.Sprintf("!!! %s !!!", s.Strong(ctx, msg))
	}
}

type Service struct {
	Shout Shout `wire:""`
}

func TestWireHandler(t *testing.T) {
	var di = picodi.New()
	di.Providers(StrongerImpl{})
	di.Providers(NewShoutHandler)

	s := Service{}
	err := di.Wire(&s)
	require.NoError(t, err)

	assert.Equal(t, "!!! HELLO !!!", s.Shout(context.Background(), "hello"))
}
