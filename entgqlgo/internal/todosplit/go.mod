module entgo.io/contrib/entgqlgo/internal/todosplit

go 1.24

require (
	entgo.io/contrib v0.0.0
	entgo.io/ent v0.14.4
	github.com/graphql-go/graphql v0.8.1
	github.com/mattn/go-sqlite3 v1.14.28
	github.com/stretchr/testify v1.10.0
)

require (
	ariga.io/atlas v0.36.2-0.20250730182955-2c6300d0a3e1 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-openapi/inflect v0.19.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/hcl/v2 v2.18.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/samber/lo v1.47.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/zclconf/go-cty v1.14.4 // indirect
	github.com/zclconf/go-cty-yaml v1.1.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Use the local contrib repo (this worktree) so the in-development entgqlgo
// extension drives codegen.
replace entgo.io/contrib => ../../../

// Target the MatthewsREIS/ent fork's split runtime layout. The classic parent
// module pins the older fork; this nested module overrides it without touching
// the parent go.mod.
replace entgo.io/ent => github.com/MatthewsREIS/ent v0.0.0-20260527160609-28e755f592ed
