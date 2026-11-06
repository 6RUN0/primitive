//go:build amd64.v3

package primitive

// isFusedFloatBuild reports a build with FMA instructions: amd64.v3 is the first
// level that has them, and the compiler then fuses x*y+z in the shape code.
const isFusedFloatBuild = true
