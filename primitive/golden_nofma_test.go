//go:build !amd64.v3

package primitive

// isFusedFloatBuild reports a build with FMA instructions, see golden_fma_test.go.
const isFusedFloatBuild = false
