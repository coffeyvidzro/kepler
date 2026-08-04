package event

import (
	"fmt"
	"slices"
	"strings"
)

type Type string

type Definition struct {
	Type             Type
	ObjectType       string
	ObjectIDRequired bool
	Subscribable     bool
}

const (
	TypeSMSSubmitted   Type = "sms.submitted"
	TypeSMSSent        Type = "sms.sent"
	TypeSMSDelivered   Type = "sms.delivered"
	TypeSMSUndelivered Type = "sms.undelivered"
	TypeSMSFailed      Type = "sms.failed"

	TypeEmailSubmitted           Type = "email.submitted"
	TypeEmailDelivered           Type = "email.delivered"
	TypeEmailDelayed             Type = "email.delayed"
	TypeEmailBounced             Type = "email.bounced"
	TypeEmailComplained          Type = "email.complained"
	TypeEmailRejected            Type = "email.rejected"
	TypeEmailFailed              Type = "email.failed"
	TypeEmailOpened              Type = "email.opened"
	TypeEmailClicked             Type = "email.clicked"
	TypeEmailSubscriptionChanged Type = "email.subscription_changed"

	TypeVerificationCreated            Type = "verification.created"
	TypeVerificationDispatched         Type = "verification.dispatched"
	TypeVerificationApproved           Type = "verification.approved"
	TypeVerificationIncorrect          Type = "verification.incorrect"
	TypeVerificationResent             Type = "verification.resent"
	TypeVerificationExpired            Type = "verification.expired"
	TypeVerificationDeliveryFailed     Type = "verification.delivery_failed"
	TypeVerificationMaxAttemptsReached Type = "verification.max_attempts_reached"
	TypeVerificationCanceled           Type = "verification.canceled"

	TypeWebhookTest Type = "webhook.test"
)

var definitions = []Definition{
	{Type: TypeSMSSubmitted, ObjectType: "sms", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeSMSSent, ObjectType: "sms", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeSMSDelivered, ObjectType: "sms", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeSMSUndelivered, ObjectType: "sms", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeSMSFailed, ObjectType: "sms", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailSubmitted, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailDelivered, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailDelayed, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailBounced, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailComplained, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailRejected, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailFailed, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailOpened, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailClicked, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeEmailSubscriptionChanged, ObjectType: "email", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationCreated, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationDispatched, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationApproved, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationIncorrect, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationResent, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationExpired, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationDeliveryFailed, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationMaxAttemptsReached, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeVerificationCanceled, ObjectType: "verification", ObjectIDRequired: true, Subscribable: true},
	{Type: TypeWebhookTest, ObjectType: "webhook_endpoint", ObjectIDRequired: true},
}

var definitionsByType = func() map[Type]Definition {
	catalog := make(map[Type]Definition, len(definitions))
	for _, definition := range definitions {
		catalog[definition.Type] = definition
	}
	return catalog
}()

func Definitions() []Definition {
	return slices.Clone(definitions)
}

func Lookup(eventType Type) (Definition, bool) {
	definition, ok := definitionsByType[eventType]
	return definition, ok
}

func ParseType(value string) (Type, error) {
	eventType := Type(strings.TrimSpace(value))
	if _, ok := Lookup(eventType); !ok {
		return "", fmt.Errorf("unknown event type %q", value)
	}
	return eventType, nil
}

func SubscribableTypes() []string {
	types := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Subscribable {
			types = append(types, string(definition.Type))
		}
	}
	return types
}

func IsSubscribable(eventType Type) bool {
	definition, ok := Lookup(eventType)
	return ok && definition.Subscribable
}
