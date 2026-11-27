// Command agree reads the document `gragas plugin contract --json` prints from standard input and
// is red unless this SDK's contract number and features are the ones in it (the push tier's check).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/birdtable/gragas-plugin-sdk/contract"
)

func main() {
	doc, err := io.ReadAll(os.Stdin)
	if err == nil {
		err = contract.Agree(doc)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "contract: "+err.Error())
		os.Exit(1)
	}
	fmt.Println("contract: the SDK and the pinned gragas agree on the contract number and the features")
}
