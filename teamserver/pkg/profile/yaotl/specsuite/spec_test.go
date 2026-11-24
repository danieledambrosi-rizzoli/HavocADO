package spectests

import (
    "testing"
)

func TestMain(m *testing.M) {
    // The specsuite test requires hcldec and hclspecsuite binaries which are
    // part of the upstream HashiCorp HCL v2 but not included in this fork.
    // Skip the entire package.
    m.Run()
}

func TestSpec(t *testing.T) {
    t.Skip("specsuite requires external hcldec/hclspecsuite binaries not present in this repo")
}
