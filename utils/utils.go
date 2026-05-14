package utils

import (
	"crypto/rand"
	"errors"
	"regexp"
)

const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

func GetRandString(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid length")
	}

	b := make([]byte, length)
	rb := make([]byte, length)

	_, err := rand.Read(rb)
	if err != nil {
		return "", err
	}

	for i := 0; i < length; i++ {
		b[i] = chars[int(rb[i])%len(chars)]
	}

	return string(b), nil
}

func IsEmailValid(email string) bool {
	re := regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
	return re.MatchString(email)
}

func GetRandCode6() (string, error) {
	const digits = "0123456789"
	const length = 6

	rb := make([]byte, length)
	_, err := rand.Read(rb)
	if err != nil {
		return "", err
	}

	b := make([]byte, length)
	for i := 0; i < length; i++ {
		b[i] = digits[int(rb[i])%len(digits)]
	}

	return string(b), nil
}
