# picodi
A tiny Dependency Injection framework in Go

With the advent of plugins (Go 1.8), DI _might_ be usefull.
We could ask the plugin to wire itself with the supplied Depency PicoDI.
This way context could evolve independently in the main program and in the plugins.

With PicoDI we can concentrate all the configuration in one place.

## How to

Considering the struct

```go
type Foo struct {
    name string
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
