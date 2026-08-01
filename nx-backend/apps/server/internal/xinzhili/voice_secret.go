package xinzhili

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	voiceSecretCipherVersion = "v1"
	voiceSecretKeyBytes      = 32
)

var (
	ErrVoiceSecretKeyInvalid        = errors.New("芯之力音色密钥配置无效：XINZHILI_SECRET_KEY 必须是 base64 编码的 32 字节密钥")
	ErrVoiceSecretCiphertextInvalid = errors.New("芯之力音色密文无效")
)

type VoiceSecretCodec struct {
	aead cipher.AEAD
}

func NewVoiceSecretCodec(base64Key string) (*VoiceSecretCodec, error) {
	base64Key = strings.TrimSpace(base64Key)
	if base64Key == "" {
		return nil, ErrVoiceSecretKeyInvalid
	}
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil || len(key) != voiceSecretKeyBytes {
		return nil, ErrVoiceSecretKeyInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("初始化芯之力音色密钥失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("初始化芯之力音色密钥失败: %w", err)
	}
	return &VoiceSecretCodec{aead: aead}, nil
}

func (c *VoiceSecretCodec) Encrypt(plaintext string) (string, error) {
	if c == nil || c.aead == nil {
		return "", ErrVoiceSecretKeyInvalid
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成芯之力音色密钥 nonce 失败: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), []byte(voiceSecretCipherVersion))
	return voiceSecretCipherVersion + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *VoiceSecretCodec) Decrypt(ciphertext string) (string, error) {
	if c == nil || c.aead == nil {
		return "", ErrVoiceSecretKeyInvalid
	}
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		return "", ErrVoiceSecretCiphertextInvalid
	}
	version, encoded, ok := strings.Cut(ciphertext, ":")
	if !ok || version != voiceSecretCipherVersion || encoded == "" {
		return "", ErrVoiceSecretCiphertextInvalid
	}
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(sealed) <= c.aead.NonceSize() {
		return "", ErrVoiceSecretCiphertextInvalid
	}
	nonce := sealed[:c.aead.NonceSize()]
	body := sealed[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, body, []byte(version))
	if err != nil {
		return "", ErrVoiceSecretCiphertextInvalid
	}
	return string(plaintext), nil
}
