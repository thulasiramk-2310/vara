package commands

import (
	"fmt"

	"github.com/thulasiramk-2310/vara/pkg/gc"
)

// RunGC implements `vara gc` (RFC-0014 §12). With apply=false it reports what
// would be reclaimed without deleting anything.
func RunGC(ctx *Context, apply bool) (string, error) {
	res, err := gc.Collect(ctx.Repository.VaraDir, apply)
	if err != nil {
		return "", err
	}
	verb := "would remove"
	if apply {
		verb = "removed"
	}
	return fmt.Sprintf(
		"Scanned %d objects, %d reachable.\n%s %d unreferenced object(s), %s.\n",
		res.Scanned, res.Reachable, verb, res.Removed, humanBytes(res.BytesFreed),
	), nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
