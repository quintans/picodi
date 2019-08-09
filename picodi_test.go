package picodi

import (
	"testing"
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
	Foo    Foo   `wire:"foo"`
	Foo2   Foo   `wire:""`
	Other  Namer `wire:"foo"`
	inner  *Foo  `wire:"fooptr"`
	inner2 Foo   `wire:"foo"`
	Fun    Foo   `wire:"foofn"`
	FooPtr *Foo  `wire:"fooptr"`
}

func (b *Bar) SetInner(v *Foo) {
	b.inner = v
}

func TestStructWire(t *testing.T) {
	pico := New()
	pico.Provider("fooptr", &Foo{"Foo"})
	pico.Provider("foo", Foo{"Foo"})
	pico.Provider("foofn", func() Foo {
		return Foo{"FooFn"}
	})
	pico.Provider("", Foo{"Foo"})

	var bar = Bar{}
	if err := pico.Wire(&bar); err != nil {
		t.Fatal("Unexpected error when wiring bar: ", err)
	}

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

	if bar.Fun.Name() != "FooFn" {
		t.Fatal("Expected \"FooFn\" for Fun, got", bar.Fun.Name())
	}
}

type Faulty struct {
	bar Bar `wire:"missing"`
}

func TestErrorWire(t *testing.T) {
	var pico = New()
	if err := pico.Wire(&Faulty{}); err == nil {
		t.Fatal("Expected error for missing provider, nothing")
	}
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
}

func (g GreeterImpl) Greet() Message {
	return g.Message
}

func NewGreeter(m Message) Greeter {
	return GreeterImpl{Message: m}
}

type Event struct {
	Greeter Greeter
}

func NewEvent(g Greeter) Event {
	return Event{Greeter: g}
}

func (e Event) Start() string {
	msg := e.Greeter.Greet()
	return string(msg)
}

func TestWireConstructors(t *testing.T) {
	var pico = New()
	pico.Provider("event", NewEvent)
	pico.Provider("message", NewMessage)
	pico.Provider("greeter", NewGreeter)

	e, err := pico.Resolve("event") // will wire if not already
	if err != nil {
		t.Fatal(err)
	}

	actual := e.(Event).Start()
	if actual != "Hi there!" {
		t.Fatal("Espected \"Hi there!\", but got " + actual)
	}
}
