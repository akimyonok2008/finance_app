package strategy

import "errors"

var (
	ErrNotFound = errors.New("public strategy not found")
	ErrSelfCopy = errors.New("cannot copy your own strategy")
)
