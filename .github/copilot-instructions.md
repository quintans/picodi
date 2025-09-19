# PicoDI AI Coding Instructions

## Project Overview
PicoDI is a lightweight dependency injection framework for Go using reflection. It provides an alternative to Google's Wire by using runtime reflection instead of code generation, emphasizing simplicity and ease of use.

## Core Architecture Patterns

### Provider Registration Patterns
- **Type-based providers**: `di.Providers(instance)` registers by full type name
- **Named providers**: `di.NamedProvider("name", instance)` for multiple instances of same type
- **Function providers**: Use `func() Type` or `func() (Type, Clean, error)` for lazy instantiation
- **Transient providers**: `di.TransientProviders()` or `wire:",transient"` tag for fresh instances

### Wiring Mechanisms
1. **Struct field injection**: Use `wire:""` tag (type-based) or `wire:"name"` (named)
2. **Function injection**: `di.Wire(func(dep Dependency) {})` for constructor pattern
3. **Map injection**: `map[picodi.Named]Type` collects all named instances of a type
4. **Interface resolution**: Automatically finds first implementation for interface types

### Key Types and Interfaces
- `PicoDI`: Main container with `namedInjectors` and `typeInjectors` maps
- `AfterWirer`: Implement `AfterWire() (Clean, error)` for post-injection setup
- `Clean`: Function type for cleanup operations (database disconnections, etc.)
- `Named`: String type used as map key for named dependency collections

## Critical Validation Patterns

### Provider Function Signatures
Valid provider functions must return:
- `Type` (single return)
- `(Type, error)` (with error handling)
- `(Type, Clean)` (with cleanup)
- `(Type, Clean, error)` (full signature)

### Wire Function Signatures
Wire functions must:
- Have 1+ input parameters (dependencies)
- Return nothing or only an error
- Example: `func(dep1 Type1, dep2 Type2) error`

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
- Missing dependencies: "no provider was found for type/name"
- Multiple interface implementations: "more than one implementation was found"
- Invalid provider functions: Validate return type count and types

### Unsafe Field Access
For unexported fields without setters, uses `unsafe.Pointer` and `reflect.NewAt()` to bypass Go's access controls. Prefer public fields or setter methods.

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

### Interface Resolution
- Single implementation: Auto-resolved
- Multiple implementations: Use named providers to disambiguate
- No implementations: Clear error message with type information

### Reflection Limitations
- Struct fields must be settable or have `SetFieldName()` methods
- Function validation occurs at registration, not call time
- Type information preserved through `reflect.Type` in injector structs