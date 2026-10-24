package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

const (
	jwksMaxAge      = time.Hour
	jwksMinInterval = 30 * time.Second 
)

type jwksCache struct {
	url    string
	client *http.Client

	mu          sync.RWMutex
	keys        map[string]any
	fetchedAt   time.Time
	lastAttempt time.Time
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	N   string `json:"n"`
	E   string `json:"e"`
}


func (c *jwksCache) key(ctx context.Context, kid string) (any, error) {
	c.mu.RLock()
	k, ok := c.keys[kid]
	fresh := time.Since(c.fetchedAt) < jwksMaxAge
	c.mu.RUnlock()
	if ok && fresh {
		return k, nil
	}

	if err := c.refresh(ctx); err != nil && !ok {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("signing key not found")
}

func (c *jwksCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.lastAttempt) < jwksMinInterval {
		return nil
	}
	c.lastAttempt = time.Now()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching signing keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching signing keys: status %d", resp.StatusCode)
	}

	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return fmt.Errorf("reading signing keys: %w", err)
	}

	keys := make(map[string]any, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kid == "" {
			continue
		}
		pub, err := k.publicKey()
		if err != nil {
			continue 
		}
		keys[k.Kid] = pub
	}
	c.keys = keys
	c.fetchedAt = time.Now()
	return nil
}

func b64(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

func (k jwk) publicKey() (any, error) {
	switch k.Kty {
	case "EC":
		if k.Crv != "P-256" {
			return nil, errors.New("unsupported curve")
		}
		x, err := b64(k.X)
		if err != nil {
			return nil, err
		}
		y, err := b64(k.Y)
		if err != nil {
			return nil, err
		}
		if len(x) > 32 || len(y) > 32 {
			return nil, errors.New("bad EC key size")
		}
		point := make([]byte, 65)
		point[0] = 4
		copy(point[1+32-len(x):33], x)
		copy(point[33+32-len(y):], y)
		px, py := elliptic.Unmarshal(elliptic.P256(), point) 
		if px == nil {
			return nil, errors.New("EC point is not on the curve")
		}
		return &ecdsa.PublicKey{Curve: elliptic.P256(), X: px, Y: py}, nil
	case "RSA":
		n, err := b64(k.N)
		if err != nil {
			return nil, err
		}
		e, err := b64(k.E)
		if err != nil {
			return nil, err
		}
		eInt := new(big.Int).SetBytes(e)
		if !eInt.IsInt64() || eInt.Int64() < 3 || eInt.Int64() > 1<<31-1 {
			return nil, errors.New("bad RSA exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(eInt.Int64())}, nil
	}
	return nil, errors.New("unsupported key type")
}
