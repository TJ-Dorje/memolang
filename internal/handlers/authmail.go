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

func resetCodeMail(to, code string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Your MemoLang password reset code: " + code,
		Body: "Your code to choose a new password is:\n\n" +
			"    " + code + "\n\n" +
			"Type it on the page where you asked for it. It works for 15 minutes.\n\n" +
			"If you didn't ask to reset your password, ignore this email: your\n" +
			"password stays the same without this code.\n",
	}
}

func emailChangeCodeMail(to, code string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Your MemoLang code: " + code,
		Body: "Your code to use this address for your MemoLang account is:\n\n" +
			"    " + code + "\n\n" +
			"Type it on your Security page. It works for 15 minutes.\n\n" +
			"If you didn't ask for this, ignore this email.\n",
	}
}

// addressInUseMail goes to an address someone tried to move their account
// to while it already belongs to another account. The page they see is the
// usual one, so it doesn't reveal that the address is taken.
func addressInUseMail(to string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Someone tried to use your email on MemoLang",
		Body: "Someone asked to move their MemoLang account to this email address,\n" +
			"but it already belongs to your account. Nothing has changed.\n",
	}
}

// The notices below report a change that already happened, so someone who
// took over the account can't make it quietly.

func passwordChangedMail(to string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Your MemoLang password was changed",
		Body: "The password for your MemoLang account was just changed, and every\n" +
			"other session was signed out.\n\n" +
			"If this wasn't you, choose \"Forgot password?\" on the login page now\n" +
			"to set a new one.\n",
	}
}

func emailChangeRequestedMail(to, newEmail string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Your MemoLang email is being changed",
		Body: "Someone logged in to your MemoLang account asked to change its email\n" +
			"address to " + newEmail + ". It changes once a code sent there is entered.\n\n" +
			"If this wasn't you, log in, cancel the change on the Security page and\n" +
			"change your password.\n",
	}
}

func emailChangedMail(to, newEmail string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Your MemoLang email was changed",
		Body: "Your MemoLang account now uses " + newEmail + ". This address no longer\n" +
			"logs in.\n\n" +
			"If this wasn't you, someone else controls your account. Contact the\n" +
			"person who runs this MemoLang to get it back.\n",
	}
}
