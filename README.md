# PicoDI

A lightweight dependency injection framework for Go using reflection.

PicoDI replicates the behavior of [Google Wire](https://github.com/google/wire) but uses runtime reflection instead of code generation, emphasizing simplicity and ease of use.

## Key Features

- **Runtime reflection**: No code generation required
- **Singleton by default**: Automatic singleton management without extra code
- **Simple API**: Just define providers and wire dependencies
- **Interface resolution**: Automatic resolution to implementations
- **Named providers**: Support for multiple instances of the same type
- **Cleanup support**: Built-in resource cleanup mechanisms
- **Dry run validation**: Test dependency configuration without instantiation

Since dependency injection typically occurs at boot time, the performance difference between reflection and code generation has minimal impact on overall application performance.

## Quick Start

Consider the following structs:

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

To inject `Foo` into `Bar`, we define providers:

```go
di := picodi.New()
di.Providers(&Foo{"Foo"})
di.Providers(&Bar{}) // Foo field will be automatically injected
```

Then we wire the dependencies:

```go
baz := Baz{}
di.Wire(&baz) // Traverses and wires all dependencies
```

This wiring approach works with both structs and functions, as shown in the examples below.

## Constructor Dependency Injection

You can use functions for constructor-style dependency injection:

```go
type Foo struct {
    Name string
}

type Bar struct {
    Foo Foo // No wire tag needed when using constructor injection
}
```

Define providers using functions:

```go
di := picodi.New()
// Provider functions can have parameters for dependency injection
di.Providers(func() Foo {
    return Foo{"Foo"}
})

bar := Bar{}
di.Wire(func(foo Foo) {
    bar.Foo = foo
})
```

**Note**: The way you wire dependencies is independent of how you define providers. Multiple calls to `Wire()` will always inject the same `Foo` instance (singleton behavior by default).

Interfaces are automatically resolved to the first implementation found that satisfies the interface.

## Named Providers

Sometimes you need multiple instances of the same type (e.g., two database connections using the same driver). Named providers solve this problem.

Consider these structs:

```go
type Foo struct {
    Name string
}

type Bar struct {
    Source Foo
    Sink   Foo
}
```

Create named providers:

```go
di := picodi.New()
di.NamedProviders(picodi.NamedProviders{
    "source": "SOURCE",
    "sink":   "SINK",
    "other":  1, // This won't be included in the string map below
})

// Equivalent to:
// di.NamedProvider("source", "SOURCE")
// di.NamedProvider("sink", "SINK")
// di.NamedProvider("other", 1)
```

Wire using a map:

```go
bar := Bar{}
// The map key type is picodi.Named
// m will contain all 'string' type providers
// Other types require separate Wire() calls
di.Wire(func(m map[picodi.Named]string) {
    bar.Source = m["source"]
    bar.Sink = m["sink"]
})
```

## Struct Field Injection

Tag struct fields with the provider name to enable automatic named injection:

```go
type Bar struct {
    Source Foo `wire:"source"`
    Sink   Foo `wire:"sink"`
}
```

> **Note**: If no value is specified for the wire tag (`wire:""`), the search will be done by type instead of name.

Execute the wiring:

```go
bar := Bar{}
di.Wire(&bar)
```

### Unexported Fields

If a field is tagged with `wire` but is unexported, PicoDI will look for a setter method. For example, for a field named `xpto string`, it will look for `SetXpto(xpto string)`. If no setter exists, it will write directly to the field (though this should be avoided when possible).

### AfterWire Hook

If a struct implements the `AfterWirer` interface, the `AfterWire() (Clean, error)` method will be called after all fields are set, providing an opportunity for bootstrapping, validation, etc.

```go
func (b *Bar) AfterWire() (picodi.Clean, error) {
    // Perform post-injection setup
    return nil, nil
}
```

## Interface Resolution

PicoDI supports dependency injection with interfaces and functions. Consider this example with three interdependent types:

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

> `GreeterImpl` implements the `Greeter` interface

Declare providers using functions:

```go
di := picodi.New()
di.Providers(func() Message {
    return Message("Hello, World!")
})

// This provider receives an interface parameter
// PicoDI will inject the first implementation it finds
di.Providers(func(m Message) GreeterImpl {
    return GreeterImpl{Message: m}
})

di.NamedProvider("event", func(g Greeter) Event {
    return Event{Greeter: g}
})
```

You can inject into a target structure using `di.Wire()` or request a specific named type:

```go
event, _ := di.Resolve("event") // Lazily wires dependencies
```

## Transient Dependencies

By default, PicoDI uses singleton behavior. To force fresh instances on each injection, use the `transient` flag with factory providers.

> **Performance Note**: Since PicoDI relies heavily on reflection, transient dependencies will impact performance more than singletons.

```go
type Bar struct {
    Foo Foo `wire:",transient"`
}
```

```go
di := picodi.New()
di.Providers(func() Foo {
    return Foo{"Foo"}
})

bar := Bar{}
di.Wire(&bar)
di.Wire(&bar) // bar.Foo will be a different instance from the previous call
```

## Resource Cleanup

For proper resource management (like database disconnections), providers can return a cleanup function of type `picodi.Clean`. When wiring, you receive a global cleanup function that handles all registered cleanup operations.

```go
di := picodi.New()
di.Providers(func() (Foo, picodi.Clean) {
    foo := Foo{"Foo"}
    return foo, func() {
        fmt.Println("Cleaning up Foo resource")
    }
})

bar := Bar{}
clean, err := di.Wire(&bar)
if err != nil {
    log.Fatal(err)
}

// Use your dependencies...

// Cleanup when done
clean()
```

## Validation with Dry Run

A disadvantage of using reflection is that configuration errors are only discovered at runtime. To mitigate this, use the `DryRun()` method in tests to validate your dependency configuration.

This method validates the configuration without actually running the providers, so no services need to be running.

```go
func TestDIConfig(t *testing.T) {
    di := setupMyDI() // Your DI configuration
    
    service := Service{}
    err := di.DryRun(&service)
    require.NoError(t, err)
}
```

This approach helps catch configuration issues early in your development cycle.

## License

This project is licensed under the terms specified in the LICENSE file.
