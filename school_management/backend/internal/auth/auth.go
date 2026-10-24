package auth

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Subject string 
	Email   string
}

type tokenClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

type Verifier struct {
	secret []byte
	jwks   *jwksCache
	parser *jwt.Parser
}

func NewVerifier(supabaseURL, secret string, client *http.Client) *Verifier {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	v := &Verifier{secret: []byte(secret)}
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256", "ES256", "RS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithAudience("authenticated"),
		jwt.WithLeeway(5 * time.Second),
	}
	if supabaseURL != "" {
		v.jwks = &jwksCache{url: supabaseURL + "/auth/v1/.well-known/jwks.json", client: client}
		opts = append(opts, jwt.WithIssuer(supabaseURL+"/auth/v1"))
	}
	v.parser = jwt.NewParser(opts...)
	return v
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	var tc tokenClaims
	_, err := v.parser.ParseWithClaims(token, &tc, func(t *jwt.Token) (any, error) {
		switch t.Method.Alg() {
		case "HS256":
			if len(v.secret) == 0 {
				return nil, errors.New("HS256 tokens are not accepted by this server")
			}
			return v.secret, nil
		case "ES256", "RS256":
			if v.jwks == nil {
				return nil, errors.New("asymmetric tokens are not accepted by this server")
			}
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				return nil, errors.New("token has no key id")
			}
			return v.jwks.key(ctx, kid)
		}
		return nil, errors.New("unsupported signing algorithm")
	})
	if err != nil {
		return nil, err
	}
	if !uuidRe.MatchString(tc.Subject) {
		return nil, errors.New("token subject is not a valid user id")
	}
	return &Claims{Subject: tc.Subject, Email: tc.Email}, nil
}
