package rendering

// FormatSender returns a user-friendly display name for message senders
func FormatSender(sender string) string {
	if sender == "human" {
		return "You"
	}
	return "Claude"
}
