# PicoDI AI Coding Instructions

## Project Overview
PicoDI is a lightweight dependency injection framework for Go using reflection. It provides an alternative to Google's Wire by using runtime reflection instead of code generation, emphasizing simplicity and ease of use. The framework is designed to be minimal, idiomatic Go code that follows standard library patterns.

## Core Architecture Patterns

### Provider Registration Patterns
- **Type-based providers**: `di.Providers(instance1, instance2, ...)` registers multiple instances by their full type names
- **Named providers**: `di.NamedProvider("name", instance)` for multiple instances of same type with distinct names
- **Bulk named providers**: `di.NamedProviders(map[string]any{...})` for batch registration
- **Function providers**: Use `func() Type`, `func() (Type, error)`, `func() (Type, Clean)`, or `func() (Type, Clean, error)` for lazy instantiation
- **Transient providers**: `di.TransientProviders(...)` and `di.NamedTransientProvider()` for fresh instances on each injection
- **Bulk transient providers**: `di.NamedTransientProviders(map[string]any{...})` for batch transient registration

### Wiring Mechanisms
1. **Struct field injection**: Use `wire:""` tag (type-based) or `wire:"name"` (named injection)
2. **Function injection**: `di.Wire(func(dep1 Type1, dep2 Type2) error {})` for constructor pattern
3. **Map injection**: `map[picodi.Named]Type` automatically collects all named instances of compatible types
4. **Interface resolution**: Automatically finds single implementation for interface types; errors if multiple implementations exist
5. **Generic getters**: `GetByType[T](di)` and `Resolve[T](di, name)` for type-safe access

### Key Types and Interfaces
- `PicoDI`: Main container with `namedInjectors` and `typeInjectors` maps storing `*injector` instances
- `AfterWirer`: Implement `AfterWire() (Clean, error)` for post-injection initialization and setup
- `Clean`: Function type `func()` for cleanup operations (database disconnections, file handles, etc.)
- `Named`: String type used as map key for named dependency collections
- `NamedProviders`: Type alias for `map[string]any` used in bulk registration methods

## Critical Validation Patterns

### Provider Function Signatures
Valid provider functions must return 1-3 values in these exact patterns:
- `Type` (single return value)
- `(Type, error)` (with error handling)
- `(Type, Clean)` (with cleanup function)
- `(Type, Clean, error)` (full signature with both cleanup and error)

Invalid patterns will cause registration errors during `namedProvider()` validation.

### Wire Function Signatures
Wire functions passed to `di.Wire()` must:
- Have 1+ input parameters (dependencies to inject)
- Return nothing (`func(deps...) {}`) or only an error (`func(deps...) error`)
- All input parameters must be resolvable by type or name
- Invalid signatures are caught by `validateWireFunc()` at wire time

## Testing Conventions

### DryRun Pattern
Always use `DryRun()` in tests to validate configuration without instantiation:
```go
func TestDIConfig(t *testing.T) {
    di := setupDI()
    var target TargetStruct
    err := di.DryRun(&target)
    require.NoError(t, err)
}
```

### Test Structure
- Use `require` package for assertions (from testify)
- Test both singleton behavior and transient behavior
- Validate `AfterWire()` calls with boolean flags
- Test error scenarios with missing dependencies

## Development Workflows

### Dependency Registration Order
1. Register leaf dependencies first (no dependencies)
2. Register intermediate dependencies
3. Register root dependencies last
4. Use `DryRun()` to validate configuration

### Error Handling Patterns
- Missing dependencies: `ErrProviderNotFound` - "no provider was found for type/name"
- Multiple interface implementations: `ErrMultipleProvidersFound` - "multiple providers were found for interface type"
- Duplicate registrations: `ErrProviderAlreadyExists` - "provider already exists" 
- Invalid provider functions: Validation errors during `validateProviderFunc()`
- Invalid wire functions: Validation errors during `validateWireFunc()`

### Unsafe Field Access Strategy
The framework handles unexported struct fields through a three-tier approach:
1. **Preferred**: Direct field setting if `fieldValue.CanSet()` returns true
2. **Fallback**: Setter method detection using `Set` + field name (e.g., `SetFieldName()`)
3. **Last resort**: Unsafe pointer manipulation via `reflect.NewAt()` and `unsafe.Pointer()`

This maintains compatibility while encouraging proper Go encapsulation patterns.

## Framework Design Philosophy

### Reflection-First Approach
Unlike code generation frameworks (Wire), PicoDI embraces runtime reflection for:
- **Zero build-time dependencies**: No special build steps or generated code
- **Dynamic behavior**: Providers can be registered conditionally at runtime
- **Simple debugging**: Stack traces show actual function calls, not generated code
- **Flexible architecture**: Supports complex scenarios like conditional registration and plugin systems

### Container Architecture
The `PicoDI` struct maintains two primary maps:
- `typeInjectors map[reflect.Type]*injector`: Stores type-based registrations
- `namedInjectors map[string]*injector`: Stores named registrations

Each `injector` contains:
- `provider providerFunc`: Factory function that creates instances
- `instance any`: Cached singleton instance (nil for transient)
- `clean Clean`: Cleanup function chain
- `transient bool`: Controls singleton vs transient behavior
- `typ reflect.Type`: Original provider type for interface matching

### Lazy Instantiation Strategy
All providers use lazy instantiation:
1. **Registration**: Only stores provider functions and metadata
2. **First access**: Calls provider function and caches result (singletons)
3. **Subsequent access**: Returns cached instance or creates new one (transients)
4. **Cleanup**: Maintains cleanup function chains for proper resource management

## Memory Management

### Singleton vs Transient
- Default: Singleton instances cached in injector structs
- Transient: New instance per injection, use for stateful or request-scoped objects
- Clean functions: Chained together, called in reverse dependency order

### Cleanup Chain
Clean functions form a hierarchy:
1. Provider cleanup (database connections, file handles)
2. Dependency cleanup (injected clean functions)
3. Global cleanup (returned from Wire/Resolve calls)

## Special Cases

### Interface Resolution Logic
When resolving interface types, the container searches through `typeInjectors`:
- **Single implementation**: Automatically resolved to the matching concrete type
- **Multiple implementations**: Returns `ErrMultipleProvidersFound` - use named providers with `map[picodi.Named]InterfaceType` for disambiguation
- **No implementations**: Returns `ErrProviderNotFound` with clear type information

### Map Collection Pattern
The framework supports automatic collection of named instances:
```go
map[picodi.Named]InterfaceType  // Collects all named providers that implement InterfaceType
```
This enables plugin-like architectures and service discovery patterns.

### Reflection Implementation Details
- **Type preservation**: All `reflect.Type` information stored in `injector.typ` field
- **Validation timing**: Provider functions validated at registration; wire functions validated at wire time
- **Memory safety**: Uses `reflect.NewAt()` with `unsafe.Pointer` only for unexported field access
- **Generic support**: `GetByType[T]()` and `Resolve[T]()` provide compile-time type safety over reflection-based APIs