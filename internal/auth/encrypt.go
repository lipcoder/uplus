package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// EncryptByPhone 使用 RSA 公钥加密手机号和密码，返回 Base64 编码的密文。
func EncryptByPhone(publicKey string, phone string, password string) (string, error) {
	passwordPayload := struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}{
		Identifier: strings.TrimSpace(phone),
		Password:   password,
	}

	raw, err := marshalJSONNoHTMLEscape(passwordPayload)
	if err != nil {
		return "", err
	}

	pub, err := parsePublicKey(publicKey)
	if err != nil {
		return "", err
	}

	rawBytes := []byte(raw)

	maxLen := pub.Size() - 11
	if len(rawBytes) > maxLen {
		return "", fmt.Errorf(
			"明文长度 %d 字节超过 RSA PKCS#1 v1.5 上限 %d 字节",
			len(rawBytes),
			maxLen,
		)
	}

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, pub, rawBytes)
	if err != nil {
		return "", fmt.Errorf("RSA 加密失败: %w", err)
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func marshalJSONNoHTMLEscape(v any) (string, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("JSON 序列化失败: %w", err)
	}

	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func parsePublicKey(text string) (*rsa.PublicKey, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, `\n`, "\n"))
	if text == "" {
		return nil, errors.New("公钥为空")
	}

	if block, _ := pem.Decode([]byte(text)); block != nil {
		return parseDERPublicKey(block.Bytes)
	}

	compact := strings.Join(strings.Fields(text), "")

	der, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return nil, errors.New("无法解析公钥，需传入 PEM 或 DER Base64 公钥")
	}

	return parseDERPublicKey(der)
}

func parseDERPublicKey(der []byte) (*rsa.PublicKey, error) {
	if pubAny, err := x509.ParsePKIXPublicKey(der); err == nil {
		pub, ok := pubAny.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("不是 RSA 公钥")
		}
		return pub, nil
	}

	if pub, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return pub, nil
	}

	if cert, err := x509.ParseCertificate(der); err == nil {
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("证书中的公钥不是 RSA 公钥")
		}
		return pub, nil
	}

	return nil, errors.New("无法解析 RSA 公钥")
}
