# picodi
A tiny Dependency Injection framework using reflection in Go

This more or less replicates the behaviour [wire](https://github.com/google/wire) but uses reflection instead of code generation.

Since dependency injection is usually used in boot time, I would say that the performance difference between reflection and code generation has little impact.

One advantage that this approach has is that we use singletons when doing the injection, without extra code to handle it.
Also, for property injection the code is easy to understand.

Overall the API is easier to work with. You only need to define **providers** and **wire** them.

And finally, this was fun to code :smile:

## Quick example

Consider the following

```go
type Foo struct {
    Name string
}

type Bar struct {
    Foo Foo `wire:""`
}

type Baz struct {
    Bar Bar `wire:""`
}
```

and we want to inject `Foo` into `Bar`

We define providers

```go
di := picodi.New()
di.Providers(&Foo{"Foo"})
di.Providers(&Bar{}) // not setting field Foo. It will be injected.
```

and then we wire

```go
baz := Baz{}
di.Wire(&baz) // this will traverse all dependencies and wire them
```

This wiring is not restricted to structs as we will se bellow.

## Using functions (constructor dependency injection)

Consider the following

```go
type Foo struct {
    Name string
}

type Bar struct {
    Foo Foo // there is no need to have a wire tag
}
```

```go
di := picodi.New()
// if needed, the function can have parameters that would be dependency injectd as well
di.Providers(func() Foo {
    return Foo{"Foo"}
})

bar := Bar{}
di.Wire(func(foo Foo) {
    bar.Foo = foo
})
```

The way you wire is independent from the way we define providers.

Multiple calls to wire will inject always the same Foo instance. This is the default.

Interfaces are resolved to the first implementation found that respects the interface.

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
di.NamedProviders(picodi.NamedProviders{
    "source": "SOURCE",
    "sink": "SINK",
    "other": 1, // this will not be passed in the map bellow
})
// this is equivalent:
// di.NamedProvider("source", "SOURCE")
// di.NamedProvider("sousinkrce", "SINK")
// di.NamedProvider("other", 1)
```

and then we wire

```go
bar := Bar{}
// notice the map key type: picodi.Named
// m will have all 'string' types
// other types can be applied with another Wire() call
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

If a field is tagged with `wire` but it is unexported, then we will look for a setter for the field, for example a tagged field name `xpto string` then its setter `SetXpto(xpto string)` would be called. If there is no setter, we write directly to the field (lets avoid this situation)

If the struct implements the `AfterWirer` interface, then we call `AfterWire() (Clean, error)` after all the fields are set, giving the opportunity to do any bootstrapping, validation, etc.

```go
func (b *Bar) AfterWire() (picodi.Clean, error) {
	// after wire called
	return nil, nil
}
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

> I provide this for completenes, but since picodi relies heavely on reflection performance will be impacted.

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
di.Wire(&bar) // bar.Foo will be a differente instance from the previous call
```

## Clean up

If there is any clean up to be done, like disconnecting a database for a well behaved shutdown, the provider must return a function of type `picodi.Clean`.
When wiring we receive a global clean function that we can call to do any clean up.

```go
// ...

di.Provider(func() (Foo, picodi.Clean) {
    return Foo{"Foo"}, func() {
        fmt.Println("I am a clean up function but I don't do anything :P")
    }
}

// ...

clean, _ := di.Wire(&bar)

// do stuff

// cleaning
clean()

```

## Dry Run

A disadvantage of using reflection is that you only know if something was misconfigured when you run the application.
To mitigate this you can use the `DryRun()` method in a test to check the correctness of the configuration.

This method will not run the providers, so nothing needs to be running.

```go
func TestDIConfig(t *testing.T) {
    di := picodi.New()
    service := Service{}
    err := di.DryRun(&service)
    require.NoError(t, err)
}
```
