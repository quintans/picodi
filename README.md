# picodi
A tiny Dependency Injection framework using reflection in Go

This more or less replicates the behaviour [wire](https://github.com/google/wire) but uses reflection instead of code generation.

Since dependency injection is usually used in boot time, I would say that the performance difference between reflection and code generation has little impact.

One advantage that this approach has is that we use singletons when doing the injection, without extra code to handle it.
Also, for property injection the code is easy to understand.

Overall the API is easier to work with. You only need to define **providers** and **wire** them.

And finally this was fun to code :smile:

## Quick example

Consider the following

```go
type Foo struct {
    Name string
}

type Bar struct {
    Foo Foo
}
```

and we want to inject `Foo` into `Bar`

We create a provider for `Foo`

```go
di := picodi.New()
di.Providers(func() Foo {
    return Foo{"Foo"}
})
// di.Providers(Foo{"Foo"}) // this would yield the same result
```

> there are other ways of creating a provider

and then we wire

```go
bar := Bar{}
di.Wire(func(foo Foo) {
    bar.Foo = foo
})
```

> there are other ways of wiring

Multiple calls to wire will inject always the same Foo instance.

Interfaces are resolve to the first implementation found that respects the interface.

## Named providers

In some situations we may need two instances for the same type, for example two database connections using the same driver.

In this case we can use named providers.

Consider the following

```go
type Foo struct {
    Name string
}

type Bar struct {
    Source Foo
    Sink Foo
}
```

and we want to inject `Foo` into `Bar`

We create a provider for `Foo`

```go
di := picodi.New()
picodi.NamedProviders{
    "source": "SOURCE",
    "sink": "SINK",
    "other": 1, // this will not be passed in the map bellow
})
```

and then we wire

```go
bar := Bar{}
// notice the map key type: picodi.Named
// m will have all 'string' types  
di.Wire(func(m map[picodi.Named]string) {
    bar.Source = m["source"]
    bar.Sink = m["sink"]
})
```

## Wiring Structs

For a given struct that we are interested in wiring, we tag its fields with the name of the provider

```go
type Bar struct {
    Source Foo `wire:"source"`
    Sink Foo `wire:"sink"`
}
```

> if no value is specified for the tag key wire, `wire:""` then the search will be done on the type instead of the name

and then execute the wiring

```go
bar := Bar{}
di.Wire(&bar)
```

## Using interfaces

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
// this provider receives an interface. It will inject the first that it finds. 
// Not a good practice for the provider to return an interface, but you can do it
di.Providers(func(m Message) GreeterImpl {
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
