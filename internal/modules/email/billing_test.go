package email

import "testing"

func TestEmailRecipientCountIncludesToCCAndBCC(t *testing.T) {
	t.Parallel()

	message := validatedSend{
		To:  make([]EmailAddress, 2),
		CC:  make([]EmailAddress, 3),
		BCC: make([]EmailAddress, 4),
	}

	if count := emailRecipientCount(message); count != 9 {
		t.Fatalf("emailRecipientCount() = %d, want 9", count)
	}
}
