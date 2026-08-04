package mfa

type EnrollResponse struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

type CodeRequest struct {
	Code string `json:"code"`
}

type ConfirmResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

type StatusResponse struct {
	Enabled bool `json:"enabled"`
}
