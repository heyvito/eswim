package core

import (
	"crypto/rand"
	"github.com/cloudflare/circl/cipher/ascon"
	"sync"
)

// ASCON represents an instance capable of sealing and opening data encrypted
// by the ASCON cipher suite, using 128-bit security and 320-bit permutation
// with different round numbers, thus being enough to implement AEAD with low
// overhead.
type ASCON interface {
	// Seal seals the provided buffer and returns its ciphered data, except
	// when ASCON is initialized without a key. On the latter, the input is
	// immediately returned to the caller as-is.
	Seal(data []byte) ([]byte, error)

	// Open opens a provided sealed buffer and returns its plain data, except
	// when ASCON is initialized without a key. On the latter, the input is
	// immediately returned to the caller as-is. In case decoding fails, nil
	// is returned.
	Open(data []byte) []byte
}

// NewASCON returns a new ASCON handler. When key is empty or nil, the returned
// handler performs NOOPs on both Seal and Open operations.
func NewASCON(key []byte) (ASCON, error) {
	if len(key) == 0 {
		return &asconWrapper{enabled: false}, nil
	}

	encCipher, err := ascon.New(key, ascon.Ascon128a)
	if err != nil {
		return nil, err
	}
	decCipher, err := ascon.New(key, ascon.Ascon128a)
	if err != nil {
		return nil, err
	}

	return &asconWrapper{
		enabled:      true,
		encodeCipher: encCipher,
		decodeCipher: decCipher,
	}, nil
}

type asconWrapper struct {
	encodeMu     sync.Mutex
	encodeCipher *ascon.Cipher

	decodeMu     sync.Mutex
	decodeCipher *ascon.Cipher

	enabled bool
}

func (a *asconWrapper) Seal(data []byte) ([]byte, error) {
	if !a.enabled {
		return data, nil
	}

	a.encodeMu.Lock()
	defer a.encodeMu.Unlock()
	nonce := make([]byte, 16)
	_, err := rand.Read(nonce)
	if err != nil {
		return nil, err
	}

	cipherText := a.encodeCipher.Seal(nil, nonce, data, nil)
	return append(nonce, cipherText...), nil
}

func (a *asconWrapper) Open(data []byte) []byte {
	if !a.enabled {
		return data
	}

	a.decodeMu.Lock()
	defer a.decodeMu.Unlock()

	nonce := data[0:16]
	data = data[16:]
	result, err := a.decodeCipher.Open(nil, nonce, data, nil)
	if err != nil {
		return nil
	}

	return result
}
