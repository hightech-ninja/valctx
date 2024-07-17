# valctx

Generator of idiomatic getters and setters for context values.

## Installation

Go 1.7.6 or later is required.

### For Go version before 1.16
```shell
go get -u github.com/hightech-ninja/valctx
```

### For Go 1.16
```shell
go install github.com/hightech-ninja/valctx@latest
```

### For Go 1.17 and later
You can use `go run` command together with `//go:generate` directive.
```go
package main

//go:generate go run github.com/hightech-ninja/valctx@latest -output gen/ctx.go -package gen -field UserID:string
```

And then run:
```shell
go generate ./...
```

## Examples

### CLI
Use `-field name[:type]`. Name is required, type is optional. If type is not provided, `interface{}` is used.
For third-party and exported types use format `module/path.Type`.

Example:
```shell
valctx -output gen/ctx.go \
  -package gen \
  -field UserID \
  -field TraceIDs:[]string \
  -field ClientUUID:github.com/google/uuid.UUID
```

`gen/ctx.go`:
```go
// Code generated by valctx v0.0.1. DO NOT EDIT.

package gen

import (
    "context"
    "github.com/google/uuid"
)

type userIDKey struct{}

// Get UserID retrieves the UserID from the context.
func GetUserID(ctx context.Context) interface{} {
    v := ctx.Value(userIDKey{})
    return v
}

// SetUserID sets the UserID in the context.
func SetUserID(ctx context.Context, v interface{}) context.Context {
    return context.WithValue(ctx, userIDKey{}, v)
}

type traceIDsKey struct{}

// Get TraceIDs retrieves the TraceIDs from the context.
func GetTraceIDs(ctx context.Context) ([]string, bool) {
    v, ok := ctx.Value(traceIDsKey{}).([]string)
    return v, ok
}

// SetTraceIDs sets the TraceIDs in the context.
func SetTraceIDs(ctx context.Context, v []string) context.Context {
    return context.WithValue(ctx, traceIDsKey{}, v)
}

type clientUUIDKey struct{}

// Get ClientUUID retrieves the ClientUUID from the context.
func GetClientUUID(ctx context.Context) (uuid.UUID, bool) {
    v, ok := ctx.Value(clientUUIDKey{}).(uuid.UUID)
    return v, ok
}

// SetClientUUID sets the ClientUUID in the context.
func SetClientUUID(ctx context.Context, v uuid.UUID) context.Context {
    return context.WithValue(ctx, clientUUIDKey{}, v)
}

```
