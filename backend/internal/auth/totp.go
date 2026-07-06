package auth

import "github.com/pquerna/otp/totp"

func GenerateTOTPSecret(issuer, accountName string) (secret, otpauthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func ValidateTOTPCode(secret, code string) bool {
	return totp.Validate(code, secret)
}
