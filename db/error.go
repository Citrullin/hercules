package db

import "errors"

var ErrTransactionTooBig = errors.New("transaction too big")
var ErrTransactionConflict = errors.New("transaction conflict")
