package errors

import "fmt"

var ErrRecursiveLevelExceeded = fmt.Errorf("recursive message level exceeded")
