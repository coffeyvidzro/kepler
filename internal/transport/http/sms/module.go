package sms

import smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"

type Service = smsmodule.Service
type ListRequest = smsmodule.ListRequest
type SendRequest = smsmodule.SendRequest
type BatchSendRequest = smsmodule.BatchSendRequest
type UpdateRequest = smsmodule.UpdateRequest

var Responses = smsmodule.Responses
var SendResponses = smsmodule.SendResponses
