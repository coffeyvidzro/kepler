package outbox

import "errors"

var ErrClaimLost = errors.New("outbox event claim was lost")
