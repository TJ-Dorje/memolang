package handlers

import "memolang/internal/mail"

// The app's emails, all plain text and all without links: a code is typed
// into the page the user already has open, so a mail never has to name the
// site's address and link scanners have nothing to consume.

func verifyCodeMail(to, code string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Your MemoLang code: " + code,
		Body: "Your code to confirm this email address is:\n\n" +
			"    " + code + "\n\n" +
			"Type it on the page where you signed up. It works for 15 minutes.\n\n" +
			"If you didn't sign up for MemoLang, ignore this email: no account is\n" +
			"created without this code.\n",
	}
}

// accountExistsMail goes to the owner of a verified account when someone
// registers with their address. Usually that someone is the owner, who
// forgot they had an account.
func accountExistsMail(to string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Someone tried to register with your email",
		Body: "Someone just tried to create a MemoLang account with this email\n" +
			"address, but you already have one.\n\n" +
			"If it was you: log in instead. If you've forgotten your password,\n" +
			"choose \"Forgot password?\" on the login page.\n\n" +
			"If it wasn't you, you can ignore this email: nothing has changed on\n" +
			"your account.\n",
	}
}
