package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
)

type Alg uint8

const (
	AESGCM Alg = 1

	ChaCha20Poly1305  Alg = 2
	XChaCha20Poly1305 Alg = 3
)

type KDFParams struct {
	// scrypt 参数：越大越慢越安全（也更耗 CPU/内存）
	// 这里给一套偏“通用后端”的默认值；你也可以按设备能力调。
	N       int // CPU/memory cost，必须是 2^k 且 > 1
	R       int // block size
	P       int // parallelization
	KeyLen  int // 32 for AES-256 / ChaCha20
	SaltLen int
}

var DefaultKDF = KDFParams{
	N:       1 << 15, // 32768
	R:       8,
	P:       1,
	KeyLen:  32,
	SaltLen: 16,
}

type Option func(*options)

type options struct {
	kdf KDFParams
	// 预留：将来可加 AAD、rand reader、版本等
}

func WithKDF(p KDFParams) Option {
	return func(o *options) { o.kdf = p }
}

// EncryptWithPassword：一行密码加密（输出为二进制包，含 salt/nonce/alg 信息）
func EncryptWithPassword(alg Alg, password string, plaintext []byte, opts ...Option) ([]byte, error) {
	o := &options{kdf: DefaultKDF}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}

	if password == "" {
		return nil, errors.New("password is empty")
	}

	aead, nonceLen, err := newAEAD(alg, nil) // 先拿 nonceLen
	if err != nil {
		return nil, err
	}
	_ = aead // 这里先不用

	salt := make([]byte, o.kdf.SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key, err := scrypt.Key([]byte(password), salt, o.kdf.N, o.kdf.R, o.kdf.P, o.kdf.KeyLen)
	if err != nil {
		return nil, err
	}

	aead, nonceLen, err = newAEAD(alg, key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	// wire format:
	// magic(4) + ver(1) + alg(1) + saltLen(1) + nonceLen(1) + salt + nonce + ciphertext
	out := make([]byte, 0, 4+1+1+1+1+len(salt)+len(nonce)+len(ciphertext))
	out = append(out, 'C', 'X', 'P', 'W') // CryptoX PassWord
	out = append(out, 1)                  // version
	out = append(out, byte(alg))
	out = append(out, byte(len(salt)))
	out = append(out, byte(len(nonce)))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// DecryptWithPassword：一行密码解密（自动从数据里读出 alg/salt/nonce）
func DecryptWithPassword(password string, data []byte, opts ...Option) ([]byte, error) {
	o := &options{kdf: DefaultKDF}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}

	if password == "" {
		return nil, errors.New("password is empty")
	}
	if len(data) < 4+1+1+1+1 {
		return nil, errors.New("ciphertext too short")
	}
	if data[0] != 'C' || data[1] != 'X' || data[2] != 'P' || data[3] != 'W' {
		return nil, errors.New("invalid magic header")
	}
	ver := data[4]
	if ver != 1 {
		return nil, fmt.Errorf("unsupported version: %d", ver)
	}

	alg := Alg(data[5])
	saltLen := int(data[6])
	nonceLen := int(data[7])

	headerLen := 4 + 1 + 1 + 1 + 1
	minLen := headerLen + saltLen + nonceLen
	if saltLen <= 0 || nonceLen <= 0 || len(data) < minLen {
		return nil, errors.New("invalid salt/nonce length")
	}

	salt := data[headerLen : headerLen+saltLen]
	nonce := data[headerLen+saltLen : headerLen+saltLen+nonceLen]
	ciphertext := data[headerLen+saltLen+nonceLen:]

	key, err := scrypt.Key([]byte(password), salt, o.kdf.N, o.kdf.R, o.kdf.P, o.kdf.KeyLen)
	if err != nil {
		return nil, err
	}

	aead, _, err := newAEAD(alg, key)
	if err != nil {
		return nil, err
	}

	// gcm/open 会做认证校验：密码不对或数据被篡改会返回 error
	return aead.Open(nil, nonce, ciphertext, nil)
}

// EncryptStringWithPassword：输出 base64url（无 padding）
func EncryptStringWithPassword(alg Alg, password, plaintext string, opts ...Option) (string, error) {
	b, err := EncryptWithPassword(alg, password, []byte(plaintext), opts...)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecryptStringWithPassword：输入 base64url（无 padding）
func DecryptStringWithPassword(password, b64 string, opts ...Option) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	pt, err := DecryptWithPassword(password, data, opts...)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func newAEAD(alg Alg, key []byte) (cipher.AEAD, int, error) {
	switch alg {
	case AESGCM:
		// AES-256-GCM：key 32 bytes（也可 16/24/32，但我们统一用 32）
		if key == nil {
			// 仅用于获取 nonce size
			block, _ := aes.NewCipher(make([]byte, 32))
			g, _ := cipher.NewGCM(block)
			return g, g.NonceSize(), nil
		}
		if len(key) != 32 {
			return nil, 0, fmt.Errorf("aes-gcm key must be 32 bytes (got %d)", len(key))
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, 0, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, 0, err
		}
		return gcm, gcm.NonceSize(), nil

	case ChaCha20Poly1305:
		if key == nil {
			a, _ := chacha20poly1305.New(make([]byte, chacha20poly1305.KeySize))
			return a, a.NonceSize(), nil
		}
		if len(key) != chacha20poly1305.KeySize {
			return nil, 0, fmt.Errorf("chacha20 key must be %d bytes", chacha20poly1305.KeySize)
		}
		aead, err := chacha20poly1305.New(key)
		if err != nil {
			return nil, 0, err
		}
		return aead, aead.NonceSize(), nil

	case XChaCha20Poly1305:
		if key == nil {
			a, _ := chacha20poly1305.NewX(make([]byte, chacha20poly1305.KeySize))
			return a, a.NonceSize(), nil
		}
		if len(key) != chacha20poly1305.KeySize {
			return nil, 0, fmt.Errorf("xchacha20 key must be %d bytes", chacha20poly1305.KeySize)
		}
		aead, err := chacha20poly1305.NewX(key)
		if err != nil {
			return nil, 0, err
		}
		return aead, aead.NonceSize(), nil

	default:
		return nil, 0, fmt.Errorf("unsupported alg: %d", alg)
	}
}
