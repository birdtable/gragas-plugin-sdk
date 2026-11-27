package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Features is the features of the contract this SDK is written for, sorted. Contract 1 has none.
func Features() []string { return []string{} }

// Agree holds this SDK's contract number and features to what a gragas prints for
// `gragas plugin contract --json`: one document with the number and the sorted list of features.
// A difference is an error that names both values, so the push that finds it says what drifted.
func Agree(doc []byte) error {
	var got struct {
		Contract *int      `json:"contract"`
		Features *[]string `json:"features"`
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&got); err != nil {
		return fmt.Errorf("the pinned gragas's contract document is not one this check reads: %v", err)
	}
	if got.Contract == nil || got.Features == nil {
		return fmt.Errorf("the pinned gragas's contract document lacks the contract number or the features")
	}
	if *got.Contract != Version {
		return fmt.Errorf("contract number: the SDK is written for %d, the pinned gragas speaks %d", Version, *got.Contract)
	}
	if want := Features(); !slices.Equal(want, *got.Features) {
		return fmt.Errorf("contract features: the SDK has [%s], the pinned gragas has [%s]", strings.Join(want, ", "), strings.Join(*got.Features, ", "))
	}
	return nil
}
