package domain

// DefaultTemplates are the plain-text-and-minimal-HTML bodies this core ships
// so a fresh store can actually send mail.
//
// notification_templates was created empty and nothing seeded it, so every
// template lookup failed with "notification template … is unavailable", every
// job retried ten times and then died. A store could take orders and never send
// one confirmation, and the only trace was a dead delivery nobody queried.
//
// These are deliberately unbranded and unstyled. They are a working floor, not
// a design: a store replaces them by updating the row, and Synchronize never
// overwrites a template that already exists.
//
// Every key the system can ask for is here. A key added without a default
// belongs in this list, or the mail it describes will never leave.
var DefaultTemplates = []Template{
	{
		Key:     OrderPaidTemplate,
		Subject: "Order {{.OrderNumber}} is confirmed",
		Text:    "Thank you. We have received payment for order {{.OrderNumber}}.\n\nWe will email you again when it ships.",
		HTML:    "<p>Thank you. We have received payment for order <strong>{{.OrderNumber}}</strong>.</p><p>We will email you again when it ships.</p>",
	},
	{
		Key:     BackInStockTemplate,
		Subject: "Back in stock",
		Text:    "An item you asked about is available again.\n\nVisit the store to order it.",
		HTML:    "<p>An item you asked about is available again.</p><p>Visit the store to order it.</p>",
	},
	{
		Key:     SupportAgentReplyTemplate,
		Subject: "Re: {{.Subject}}",
		Text:    "{{.MessageBody}}\n\nReply to this ticket from your account to continue the conversation.",
		HTML:    "<p>{{.MessageBody}}</p><p>Reply to this ticket from your account to continue the conversation.</p>",
	},
	{
		// The only marketing message this core sends, so it is the one that
		// must carry a way out. UnsubscribeURL is a signed, expiring link the
		// worker refuses to send without.
		Key:     AbandonedCartTemplate,
		Subject: "You left something in your cart",
		Text:    "Your cart is still waiting.\n\nVisit the store to finish your order.\n\nTo stop receiving these emails: {{.UnsubscribeURL}}",
		HTML:    "<p>Your cart is still waiting.</p><p>Visit the store to finish your order.</p><p><a href=\"{{.UnsubscribeURL}}\">Stop receiving these emails</a></p>",
	},
	{
		Key:     ReturnApprovedTemplate,
		Subject: "Your return has been approved",
		Text:    "We have approved your return request.\n\nSend the items back and we will email you when they arrive.",
		HTML:    "<p>We have approved your return request.</p><p>Send the items back and we will email you when they arrive.</p>",
	},
	{
		Key:     ReturnRejectedTemplate,
		Subject: "About your return request",
		Text:    "We could not approve your return request.\n\nContact support if you would like to discuss it.",
		HTML:    "<p>We could not approve your return request.</p><p>Contact support if you would like to discuss it.</p>",
	},
	{
		Key:     ReturnReceivedTemplate,
		Subject: "We have received your return",
		Text:    "Your returned items have arrived.\n\nYour refund is being processed and we will email you when it is sent.",
		HTML:    "<p>Your returned items have arrived.</p><p>Your refund is being processed and we will email you when it is sent.</p>",
	},
	{
		Key:     ReturnRefundedTemplate,
		Subject: "Your refund has been sent",
		Text:    "We have sent the refund for your return.\n\nHow long it takes to appear depends on your bank.",
		HTML:    "<p>We have sent the refund for your return.</p><p>How long it takes to appear depends on your bank.</p>",
	},
}
