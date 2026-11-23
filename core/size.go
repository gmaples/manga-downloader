package core

import "fmt"

// SizeText writes a size the way people read it: 912 kB, 1.4 GB
func SizeText(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

// Bytes is a size that prints as SizeText (fmt.Sprint) and travels as a
// plain number in JSON, so the window can write it the language's way
type Bytes int64

func (b Bytes) String() string { return SizeText(int64(b)) }
