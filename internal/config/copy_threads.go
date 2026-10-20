package config

// MaxCopyThreads is the most files one copy operation works on at once.
const MaxCopyThreads = 16

// ClampCopyThreads brings a CopyThreads value into 1..MaxCopyThreads. A value
// below 1 (a typo, a negative number) means the single-file path, never "off".
func ClampCopyThreads(n int) int {
	return min(max(n, 1), MaxCopyThreads)
}
