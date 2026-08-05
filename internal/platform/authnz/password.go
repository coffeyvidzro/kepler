package authnz

import "golang.org/x/crypto/bcrypt"

const (
	passwordHashCost = bcrypt.DefaultCost

	// dummyPasswordHash is a valid bcrypt hash used when an identity has no
	// stored password. Comparing against it keeps failed login work comparable
	// without allowing the dummy credential to authenticate.
	dummyPasswordHash = "$2a$10$kjh7Ct7omWn8yjdpDkYEKesw8gYz8tWFHwL/81fTkgtZYyREI7.Uy"
)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// VerifyPassword performs one bcrypt comparison for every login attempt.
// Missing and malformed hashes are replaced with a valid dummy hash, while the
// return value remains false unless a usable stored hash was supplied.
func VerifyPassword(hash *string, password string) bool {
	candidateHash := dummyPasswordHash
	hasUsableHash := false
	if hash != nil {
		candidateHash = *hash
		if _, err := bcrypt.Cost([]byte(candidateHash)); err == nil &&
			candidateHash != dummyPasswordHash {
			hasUsableHash = true
		} else {
			candidateHash = dummyPasswordHash
		}
	}

	matches := bcrypt.CompareHashAndPassword(
		[]byte(candidateHash),
		[]byte(password),
	) == nil
	return hasUsableHash && matches
}
