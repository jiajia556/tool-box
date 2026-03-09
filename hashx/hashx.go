package hashx

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

type Alg string

const (
	MD5      Alg = "md5"
	SHA1     Alg = "sha1"
	SHA224   Alg = "sha224"
	SHA256   Alg = "sha256"
	SHA384   Alg = "sha384"
	SHA512   Alg = "sha512"
	SHA3_256 Alg = "sha3-256"
	SHA3_512 Alg = "sha3-512"
)

// Sum 一行得到 hash（hex 小写），data 支持：string / []byte / io.Reader
func Sum(alg Alg, data any) (string, error) {
	h, err := newHash(alg)
	if err != nil {
		return "", err
	}
	if err := writeAny(h, data); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// MustSum 不想处理 error 时用（出错 panic）
func MustSum(alg Alg, data any) string {
	s, err := Sum(alg, data)
	if err != nil {
		panic(err)
	}
	return s
}

// File 一行算文件 hash（hex 小写）
func File(alg Alg, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return Sum(alg, f)
}

// 便捷函数（常用）
func MD5Hex(data any) (string, error)      { return Sum(MD5, data) }
func SHA1Hex(data any) (string, error)     { return Sum(SHA1, data) }
func SHA256Hex(data any) (string, error)   { return Sum(SHA256, data) }
func SHA512Hex(data any) (string, error)   { return Sum(SHA512, data) }
func SHA3_256Hex(data any) (string, error) { return Sum(SHA3_256, data) }

// ---- internal ----

func newHash(alg Alg) (hash.Hash, error) {
	switch strings.ToLower(string(alg)) {
	case "md5":
		return md5.New(), nil
	case "sha1":
		return sha1.New(), nil
	case "sha224":
		return sha256.New224(), nil
	case "sha256":
		return sha256.New(), nil
	case "sha384":
		return sha512.New384(), nil
	case "sha512":
		return sha512.New(), nil
	case "sha3-256":
		return sha3.New256(), nil
	case "sha3-512":
		return sha3.New512(), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %q", alg)
	}
}

func writeAny(h hash.Hash, data any) error {
	switch v := data.(type) {
	case nil:
		return errors.New("data is nil")
	case []byte:
		_, err := h.Write(v)
		return err
	case string:
		_, err := h.Write([]byte(v))
		return err
	case io.Reader:
		_, err := io.Copy(h, v)
		return err
	default:
		return fmt.Errorf("unsupported data type %T (use string, []byte, io.Reader)", data)
	}
}
