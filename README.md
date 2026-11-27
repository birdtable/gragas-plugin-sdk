<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/mark/gragas-plugin-sdk-banner-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/mark/gragas-plugin-sdk-banner-light.svg">
    <img src="docs/mark/gragas-plugin-sdk-banner-light.svg" width="485" alt="gragas-plugin-sdk: the gragas feather with a plug in braces, then a coral bird in flight and the name">
  </picture>
</p>

The Go SDK that [gragas](https://github.com/birdtable/gragas) plugins are written with.

A plugin is a WebAssembly module with a `plugin.yaml` manifest. It answers gragas's five operations (`plan`, `entries`, `findings`, `rank` and `preset`) for one tool, and gragas runs it with [wazero](https://wazero.io). This repository holds the SDK those plugins link, as one Go module, `github.com/birdtable/gragas-plugin-sdk`. The plugins that ship with gragas are in [gragas-plugins](https://github.com/birdtable/gragas-plugins).

## Use

```sh
go get github.com/birdtable/gragas-plugin-sdk
```

A plugin implements `plugin.Plugin`, embeds `plugin.Unsupported` for the operations it does not provide, and calls `plugin.Serve` from `main`:

```go
package main

import (
	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/sample"
)

func main() { plugin.Serve(sample.Tidy{}) }
```

That is `sample/cmd/tidy`, a complete plugin. Build it for WebAssembly with standard Go:

```sh
GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm ./sample/cmd/tidy
```

`docs/packages.md` says what each package is for.

## Develop

`task pre-push` runs the checks (vet, vet as `linux/386`, tests, the secret scan, the contract check). Every tool the checks use is pinned in `gate/pins.env` and runs with `go run`, so nothing is installed. `task hooks` runs them on commit and push.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md). Report a vulnerability as [SECURITY.md](SECURITY.md) says.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
