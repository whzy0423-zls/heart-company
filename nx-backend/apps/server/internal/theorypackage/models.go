// Package theorypackage validates portable, review-only theory data packages.
package theorypackage

const (
	maxFiles        = 512
	maxFileBytes    = 2 << 20
	maxPackageBytes = 32 << 20
)

// Report is the non-sensitive validation result used by theorysync.
type Report struct {
	PackageID     string
	ContentDigest string
	PackageDigest string
	FileCount     int
}
