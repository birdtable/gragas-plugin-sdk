# The SDK packages

The Go SDK a plugin for gragas is written on. One module, `github.com/birdtable/gragas-plugin-sdk`. Every package but
`declarative` is standard library only, so it builds for `wasip1` with standard Go and runs natively under `go test`.

A plugin implements `plugin.Plugin`, embeds `plugin.Unsupported` for the operations it does not provide, and calls
`plugin.Serve` from `main`. The SDK reads one request from stdin, calls the operation and writes one answer to
stdout; it sorts every answer by the order rules of the plugin contract, answers `unsupported` for what it
does not know, ends a request it cannot read in a `bug` error, and names the author's own faults (a key given twice,
a mark missing) as a `bug` that says where.

| Package | Holds |
|---|---|
| `plugin` | `Serve`, `ServeIO`, the interface `Plugin`, `Unsupported`, and the errors `Unsupportedf`, `Unreadablef`, `Refusedf`, `Bugf` |
| `wire` | the types of the wire schema |
| `contract` | the contract number, the two schemas as copied, and `PIN`, the one line naming the gragas commit they were copied at |
| `sarif` | SARIF output to findings |
| `lines` | a line pattern (`{path}:{line}:{column}: {message} ({key})`) read over a tool's lines |
| `pointer` | JSON pointers, for an error's `path` |
| `scratch` | cutting the scratch folder from a printed path |
| `render` | the YAML, JSONC and TOML writers over an ordered value |
| `splice` | the text between two marker lines, replaced |
| `sample` | a sample plugin on `Serve`, `sample/cmd/tidy` its main |

The module names no tool: the plugins' folders do.

## The wire types

`wire` is hand-written to the schema's shapes until gragas's generator writes it; its output replaces
`wire/wire.go`, and the package keeps its names. `TestEveryEnumHereIsTheSchemasOwn` holds each list of names to the
pinned schema.

## Checks run here

```sh
go vet ./...
go test -count=1 ./...
GOOS=wasip1 GOARCH=wasm go build ./...
GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm ./sample/cmd/tidy
go test -run '^$' -fuzz '^FuzzParse$' -fuzztime 15s ./sarif   # likewise ./lines and ./pointer; FuzzText in ./scratch, FuzzFromJSON in ./render
```

Each parser of a tool's output (`sarif.Parse`, `lines`, `pointer.Parse`, `scratch.Text`, `render.FromJSON`) has a
fuzz target with seeds. Running the sample as a `.wasm` under gragas's `plugin test` waits on the engine's wasm host.
