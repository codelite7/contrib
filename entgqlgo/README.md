# entgqlgo

An [ent](https://entgo.io) extension that generates a code-first GraphQL API for
[github.com/graphql-go/graphql](https://github.com/graphql-go/graphql).

It is the graphql-go counterpart of [entgql](../entgql): instead of generating
`.graphql` SDL files and gqlgen bindings, it generates Go code that builds a
`graphql.Schema` at runtime. Ent's generated client does all querying — the
generated resolvers only parse GraphQL arguments, call ent, and shape results.

## Quick start

**1. Annotate your ent schema:**

```go
func (Todo) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgqlgo.RelayConnection(),
        entgqlgo.QueryField(),
        entgqlgo.Mutations(entgqlgo.MutationCreate(), entgqlgo.MutationUpdate()),
    }
}
```

**2. Register the extension in `ent/entc.go`:**

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/contrib/entgqlgo"
    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
)

func main() {
    ex, err := entgqlgo.NewExtension()
    if err != nil {
        log.Fatalf("creating entgqlgo extension: %v", err)
    }
    err = entc.Generate("./schema", &gen.Config{}, entc.Extensions(ex))
    if err != nil {
        log.Fatalf("running ent codegen: %v", err)
    }
}
```

Run codegen with `go generate ./ent/...` (the standard `//go:generate go run -mod=mod ent/entc.go` directive).

**3. Serve the generated schema:**

```go
schema, err := gqlgo.NewSchema(client)
if err != nil {
    log.Fatal(err)
}

// Using github.com/graphql-go/handler (available in go.mod):
h := handler.New(&handler.Config{Schema: &schema, Pretty: true, GraphiQL: true})
http.Handle("/graphql", h)
log.Fatal(http.ListenAndServe(":8080", nil))
```

See `internal/todo/cmd/server/main.go` for the complete runnable example.

## Annotations

The following annotations are in `entgqlgo.Annotation` and can be composed on ent
schema types and fields.

| Annotation | Applies to | Effect |
|---|---|---|
| `QueryField()` | type | Expose the type as a root Query field |
| `QueryField("name")` | type | Expose with a custom field name on Query |
| `RelayConnection()` | type | Root query field returns a Relay Connection (cursor pagination) |
| `RelayConnection()` | edge | Edge field returns a Relay Connection |
| `Mutations(MutationCreate(), MutationUpdate())` | type | Generate create/update/delete mutations |
| `MultiOrder()` | type | `orderBy` accepts a list of order terms instead of a single term |
| `OrderField("NAME")` | field | Field can be used in `orderBy` |
| `Skip(...)` | type / field | Exclude from schema; see `SkipMode` flags below |
| `Type("Name")` | field | Override the GraphQL scalar type name |
| `Unbind()` | edge | Edge name in GraphQL schema differs from the ent schema name |
| `MapsTo("name")` | edge | Map the edge to a different GraphQL field name (implies `Unbind`) |
| `UseEnumNames()` | enum field | GraphQL enum values use trimmed Go names instead of DB values |
| `Implements("Iface")` | type | Type implements a custom interface (must be registered in `CustomInterfaces`) |
| `DeprecatedEnumValues("VAL")` | enum field | Mark enum values as deprecated |

### Skip flags

`Skip` accepts any combination of the following `SkipMode` bit flags:

| Flag | Effect |
|---|---|
| `SkipType` | Skip generating the GraphQL object type entirely |
| `SkipEnumField` | Skip generating GraphQL enums for enum fields |
| `SkipOrderField` | Skip generating order input/enum for ordered fields |
| `SkipWhereInput` | Skip generating the WhereInput filter type (or exclude the field from it) |
| `SkipMutationCreateInput` | Skip generating `Create<Type>Input` (or exclude the field from it) |
| `SkipMutationUpdateInput` | Skip generating `Update<Type>Input` (or exclude the field from it) |
| `SkipAll` | All of the above |

`Skip()` with no arguments equals `SkipAll`.

### Annotation examples

**`QueryField` and `RelayConnection` on a type**

Schema:
```go
func (Todo) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgqlgo.RelayConnection(),
        entgqlgo.QueryField().Description("This is the todo item"),
        entgqlgo.Mutations(entgqlgo.MutationCreate(), entgqlgo.MutationUpdate()),
        entgqlgo.MultiOrder(),
    }
}
```

Generated Query field (Relay connection with multi-order):
```graphql
type Query {
  todos(
    after: Cursor
    first: Int
    before: Cursor
    last: Int
    orderBy: [TodoOrder!]
    where: TodoWhereInput
  ): TodoConnection!
}
```

**`QueryField` without `RelayConnection` (plain list)**

Schema (BillProduct has no `RelayConnection`):
```go
func (BillProduct) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgqlgo.QueryField(),
    }
}
```

Generated Query field:
```graphql
type Query {
  billProducts: [BillProduct!]!
}
```

**`RelayConnection` on an edge**

Schema:
```go
edge.To("children", Todo.Type).
    Annotations(entgqlgo.RelayConnection())
```

Resulting edge field returns a connection instead of a list:
```graphql
type Todo {
  children(after: Cursor, first: Int, before: Cursor, last: Int): TodoConnection!
}
```

**`OrderField` on a field**

Schema:
```go
field.Int("priority").
    Annotations(entgqlgo.OrderField("PRIORITY"))
```

Adds `PRIORITY` to the `TodoOrderField` enum and accepts it in `orderBy`.

**`MultiOrder` on a type**

Without `MultiOrder()`, `orderBy` accepts a single `{field, direction}` object.
With `MultiOrder()`, it accepts a list:

```graphql
todos(orderBy: [TodoOrder!]) ...
# vs (without MultiOrder):
todos(orderBy: TodoOrder) ...
```

**`Skip` on a field**

```go
field.Time("created_at").
    Annotations(entgqlgo.Skip(entgqlgo.SkipMutationCreateInput))
```

`created_at` remains in the GraphQL type and WhereInput but is excluded from
`CreateTodoInput`.

**`UseEnumNames` and `DeprecatedEnumValues`**

Schema:
```go
field.Enum("kind").
    NamedValues(
        "Primary",   "PRIMARY",
        "Secondary", "SECONDARY",
    ).
    Annotations(
        entgqlgo.UseEnumNames(),
        entgqlgo.DeprecatedEnumValues("Secondary"),
    )
```

Without `UseEnumNames`, enum values are `PRIMARY` / `SECONDARY` (the DB values).
With `UseEnumNames`, they become `Primary` / `Secondary` (the trimmed Go names),
and the DB value is noted in the description.

`DeprecatedEnumValues` matches against the GraphQL name as it appears in the
schema: the DB value by default (e.g. `"DISABLED"`), or the trimmed Go name
when `UseEnumNames` is set (e.g. `"Secondary"`). This matches entgql's behavior.

**`Type` — overriding the GraphQL scalar**

```go
field.String("blob").
    Annotations(entgqlgo.Type("Upload"))
```

Forces the field's GraphQL type to the named scalar instead of the default
mapping.

**`Implements` — custom interfaces**

```go
func (Category) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgqlgo.Implements("NamedNode"),
    }
}
```

The name must be registered in `CustomInterfaces` before calling `NewSchema` or
`graphql.NewSchema`; see [Implementing custom interfaces](#implementing-custom-interfaces).

## Pagination

Types and edges annotated with `RelayConnection()` follow the
[Relay Cursor Connections spec](https://relay.dev/graphql/connections.htm).

The generated connection field has the signature:

```graphql
todos(
  after: Cursor
  first: Int
  before: Cursor
  last: Int
  orderBy: TodoOrder      # [TodoOrder!] with MultiOrder()
  where: TodoWhereInput   # absent when WhereInputs are disabled
): TodoConnection!
```

The connection type includes `edges { node { ... } cursor }`, `pageInfo`
(`hasNextPage`, `hasPreviousPage`, `startCursor`, `endCursor`), and
`totalCount`.

## Filtering and ordering

`WhereInput` filter types are generated by default. To disable:

```go
ex, err := entgqlgo.NewExtension(entgqlgo.WithWhereInputs(false))
```

`OrderField` annotations add values to the `<Type>OrderField` enum. The
`orderBy` argument accepts `{field: <Field>, direction: ASC|DESC}`. With
`MultiOrder()` on the type, it accepts a list of such terms.

## Extending the schema

Use `SchemaConfig` to obtain the underlying `graphql.SchemaConfig` before
building the schema. You can add custom fields or a Subscription root:

```go
cfg := gqlgo.SchemaConfig(client)

// Add a custom query field.
cfg.Query.AddFieldConfig("ping", &graphql.Field{
    Type: graphql.NewNonNull(graphql.String),
    Resolve: func(p graphql.ResolveParams) (interface{}, error) {
        return "pong", nil
    },
})

// Add a custom mutation field.
cfg.Mutation.AddFieldConfig("clearTodos", &graphql.Field{
    Type:        graphql.NewNonNull(graphql.Int),
    Description: "Delete all todos and return the number deleted.",
    Resolve: func(p graphql.ResolveParams) (interface{}, error) {
        return client.Todo.Delete().Exec(p.Context)
    },
})

// Attach a Subscription root.
cfg.Subscription = graphql.NewObject(graphql.ObjectConfig{
    Name: "Subscription",
    Fields: graphql.Fields{
        "todoEvents": &graphql.Field{
            Type: graphql.NewNonNull(graphql.String),
            Resolve: func(p graphql.ResolveParams) (interface{}, error) {
                return p.Source, nil
            },
            Subscribe: func(p graphql.ResolveParams) (interface{}, error) {
                return myEventChannel, nil
            },
        },
    },
})

schema, err := graphql.NewSchema(cfg)
```

Subscriptions are served via `graphql.Subscribe`.

## Transactions

`WithTransactions()` wraps every **generated** mutation field in its own
database transaction:

```go
schema, err := gqlgo.NewSchema(client, gqlgo.WithTransactions())
```

For custom mutation fields, use `WithTx` directly:

```go
cfg.Mutation.AddFieldConfig("seedTodo", &graphql.Field{
    Type: graphql.NewNonNull(graphql.Boolean),
    Resolve: gqlgo.WithTx(client, func(p graphql.ResolveParams) (interface{}, error) {
        tc := gqlgo.ClientFromContext(p.Context, client)
        _, err := tc.Todo.Create().
            SetText("inside tx").
            SetStatus(todo.StatusInProgress).
            Save(p.Context)
        return err == nil, err
    }),
})
```

`ClientFromContext` returns the transactional client stored in the context by
`WithTx`, falling back to the base client when no transaction is active.

## Implementing custom interfaces

Types annotated with `Implements("InterfaceName")` will include the named
interface in their GraphQL type definition — **provided** the interface is
registered in `CustomInterfaces` before the schema is built. Names not present
in the map are silently omitted (no panic, no error).

```go
// Define the interface once (e.g. in an init() or a shared setup function).
var namedNodeInterface = graphql.NewInterface(graphql.InterfaceConfig{
    Name:        "NamedNode",
    Description: "An object with a text name.",
    Fields: graphql.Fields{
        "text": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
    },
    ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
        switch p.Value.(type) {
        case *ent.Category:
            return gqlgo.CategoryType
        default:
            return nil
        }
    },
})

func init() {
    gqlgo.CustomInterfaces["NamedNode"] = namedNodeInterface
}

// Now build the schema — Category will list NamedNode among its interfaces.
schema, err := gqlgo.NewSchema(client)
```

After registration, inline fragments on the custom interface work as expected:

```graphql
query {
  categories {
    edges {
      node {
        ... on NamedNode { text }
      }
    }
  }
}
```

## Extension options

| Option | Default | Effect |
|---|---|---|
| `WithWhereInputs(bool)` | `true` | Enable/disable generating `WhereInput` filter types |
| `WithRelaySpec(bool)` | `true` | Enable/disable generating the Relay `Node` interface and `node`/`nodes` query fields |
| `WithMapScalarFunc(fn)` | built-in mapping | Custom function mapping an ent field + operator to a GraphQL scalar name |
| `WithTemplates(...)` | all built-in templates | Replace the code generation templates entirely |

## Differences from entgql + gqlgen

| Feature | entgql + gqlgen | entgqlgo |
|---|---|---|
| Schema definition | SDL files (`.graphql`) + gqlgen codegen | Go code, `graphql.Schema` built at runtime |
| Mutation root | hand-written in `.graphql` files | generated (create / update / delete) |
| Delete mutations | not generated | generated for types with `MutationUpdate()` |
| Schema extension | extra `.graphql` files | `SchemaConfig()` + `AddFieldConfig` |
| Custom directives (`@hasPermissions`) | supported via SDL | not supported — graphql-go fields cannot carry applied directives; wrap resolvers instead |
| Query complexity limits | gqlgen runtime feature | not available |
| `@goField` / `@goModel` | gqlgen binding directives | not applicable |
| Transactions | `Transactioner` middleware — one transaction per GraphQL operation | `WithTransactions()` — one transaction per mutation **field**; a request with multiple mutation fields runs independent transactions |

## Regenerating the examples

```bash
cd internal/todo && go generate ./...
cd internal/parity && go generate ./...
```

Templates in `template/` are the source of truth. Never edit files under
`internal/*/ent/gqlgo/` by hand — they are overwritten on every `go generate`.
