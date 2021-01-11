# picodi
A tiny Dependency Injection framework using reflection in Go

This more or less replicates the behaviour [wire](https://github.com/google/wire) but uses reflection instead of code generation.

Since dependency injection is usually used in boot time, I would say that the performance difference between reflection and code generation has little impact.

One advantage that this approach has is that we use singletons when doing the injection, without extra code to handle it.
Also, for property injection the code is easy to understand.

Overall the API is easier to work with. You only need to define providers and wire them.

And finally this was fun to code :smile:

## Quick example

Consider the following

```go
type Foo struct {
    Name string
}
```

we declare a named provider for that type

```go
di := picodi.New()
di.NamedProvider("foo", Foo{"Foo"})
```

> there are other ways of declaring a provider

For a given struct that we are interested in wiring, we tag its fields with the name of the provider

```go
type Bar struct {
    Foo Foo `wire:"foo"`
}
```

and then execute the wiring

```go
bar := Bar{}
di.Wire(&bar)
```

This also would work if the target type was an interface that the provided type implemented.

For a given name, or type when no name is provided, the same instance is used. If we call `di.Wire(&bar)` the same exact instance would be injected.

## Factories

We can also use dependency injection with functions.

Consider the following example were we have 3 types that depend on one another.

```go
type Message string

type Greeter interface {
    Greet() Message
}

type GreeterImpl struct {
    Message Message
}

func (g GreeterImpl) Greet() Message {
    return g.Message
}

type Event struct {
    Greeter Greeter
}

func (e Event) Start() string {
    msg := e.Greeter.Greet()
    return string(msg)
}
```

> `GreeterImpl` implements `Greeter`

like before, we declare the providers, but this time we use functions.

```go
di.NamedProvider("event", func(g Greeter) Event {
    return Event{Greeter: g}
})
di.Provider(func() Message {
    return Message("Hi there!")
})
// Not a good practice for the provider to return an interface, but you can do it  
di.Provider(func(m Message) Greeter {
    return GreeterImpl{Message: m}
})
```

And we could inject to a target structure using `di.Wire` like before or ask explicitly for the named type

```go
event, _ := di.Resolve("event") // will lazily wire
```

## Wire with function

```go
var di = picodi.New()
di.NamedProviders(picodi.NamedProviders{
    "message": NewMessage,
    "greeter": NewGreeter,
})

evt := Event{}
err := di.Wire(func(g GreeterImpl) {
    evt.Greeter = g
})
```

If we the same type to be injected but different instances, for example for connection for two databases, 
and if the struct wire tag is not possible to use, we can use named factories injections.

```go
func TestWireFuncByName(t *testing.T) {
	di := picodi.New()
	di.NamedProviders(picodi.NamedProviders{
		"message1": "hello",
		"message2": "world",
		"message3": 1, // this will not be used injected bellow
	})

	// only strings will passed to factory
	err := di.Wire(func(m map[picodi.Named]string) {
		m1 := m["message1"]
        m2 := m["message2"]
        // ...
    })
}
```

> notice that the key type of the map is `picodi.Named`

## Transient

To force fresh instance to be injected we need to use the flag `transient` and use a factory provider.

```go
type Bar struct {
    Foo Foo `wire:",transient"`
}
```

```go
di := picodi.New()
di.Provider(func() Foo {
    return Foo{"Foo"}
}

bar := Bar{}
di.Wire(&bar)
di.Wire(&bar) // bar.Foo will be different from the previous call
```
