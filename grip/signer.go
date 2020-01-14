package grip

import (
	"time"

	"github.com/square/go-jose/v3"
	"github.com/square/go-jose/v3/jwt"
)

type Signer struct {
	issuer string
	signer jose.Signer
}

func NewSigner(issuer string, signer jose.Signer) *Signer {
	return &Signer{
		issuer: issuer,
		signer: signer,
	}
}

func (s Signer) Sign(exp time.Time) (string, error) {
	return jwt.Signed(s.signer).Claims(map[string]interface{}{
		"iss": s.issuer,
		"exp": exp.Unix(),
	}).CompactSerialize()
}
