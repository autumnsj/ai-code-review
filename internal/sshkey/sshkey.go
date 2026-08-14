// Package sshkey 生成与解析 OpenSSH ed25519 密钥对。
package sshkey

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// KeyPair 持有一个 SSH 密钥对的三种表示。
type KeyPair struct {
	PrivatePEM  string // OpenSSH 私钥 PEM 文本
	Public      string // OpenSSH authorized_keys 公钥行
	Fingerprint string // SHA256 指纹，形如 SHA256:xxxx
}

// GenerateEd25519 生成一把全新的 ed25519 密钥对。
func GenerateEd25519() (*KeyPair, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519: %w", err)
	}
	return fromSigner(priv)
}

// ParsePrivateKey 解析用户粘贴的 OpenSSH/PEM 私钥，校验其可用并派生出公钥行与指纹。
// 私钥原文直接保留返回（不重新序列化）。不支持带口令加密的私钥。
func ParsePrivateKey(pemBytes []byte) (*KeyPair, error) {
	if block, _ := pem.Decode(pemBytes); block != nil {
		if strings.Contains(block.Headers["DEK-Info"], "ENCRYPTED") {
			return nil, errors.New("暂不支持带口令加密的私钥，请提供未加密的私钥")
		}
	}
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败（请粘贴完整的 OpenSSH/PEM 私钥）：%w", err)
	}
	return &KeyPair{
		PrivatePEM:  strings.TrimSpace(string(pemBytes)),
		Public:      strings.TrimRight(string(ssh.MarshalAuthorizedKey(signer.PublicKey())), "\n"),
		Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
	}, nil
}

func fromSigner(priv crypto.PrivateKey) (*KeyPair, error) {
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("new signer: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	return &KeyPair{
		PrivatePEM:  string(pem.EncodeToMemory(block)),
		Public:      strings.TrimRight(string(ssh.MarshalAuthorizedKey(signer.PublicKey())), "\n"),
		Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
	}, nil
}
