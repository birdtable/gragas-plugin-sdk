// Command tidy is the sample plugin as a program: one call, one request on stdin, one answer on stdout. Built for
// wasip1 it is the module a gragas "plugin test" runs (GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm).
package main

import (
	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/sample"
)

func main() { plugin.Serve(sample.Tidy{}) }
