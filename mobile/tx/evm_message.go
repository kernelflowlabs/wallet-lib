//go:build evm || !custom

package tx

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/evm"
)

type EvmMessageSigner struct {
	message []byte
	err     error
}

func NewEvmMessageSigner() *EvmMessageSigner {
	return &EvmMessageSigner{}
}

func (s *EvmMessageSigner) SetUtf8(text string) *EvmMessageSigner {
	s.message = []byte(text)
	s.err = nil
	return s
}

func (s *EvmMessageSigner) SetHex(hexMessage string) *EvmMessageSigner {
	raw, err := hex.DecodeString(strings.TrimPrefix(hexMessage, "0x"))
	if err != nil {
		s.message = nil
		s.err = fmt.Errorf("invalid_input: message is not hex")
		return s
	}
	s.message = raw
	s.err = nil
	return s
}

func (s *EvmMessageSigner) SetBytes(message []byte) *EvmMessageSigner {
	s.message = append([]byte(nil), message...)
	s.err = nil
	return s
}

func (s *EvmMessageSigner) Digest() string {
	if s.err != nil {
		return ""
	}
	return "0x" + hex.EncodeToString(evm.PersonalSignDigest(s.message))
}

func (s *EvmMessageSigner) Sign(privateKey []byte) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	sig, err := evm.PersonalSign(privateKey, s.message)
	WipeByte(privateKey)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(sig), nil
}

func (s *EvmMessageSigner) SignWithSession(handle string) (string, error) {
	return s.SignWithSessionExpecting(handle, "")
}

func (s *EvmMessageSigner) SignWithSessionExpecting(handle, expectedDigest string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if err := matchesExpectedDigest(s.Digest(), expectedDigest); err != nil {
		return "", err
	}
	var signature string
	err := session.WithKey(handle, session.ChainEVM, func(privateKey []byte, _ string) error {
		sig, signErr := evm.PersonalSign(privateKey, s.message)
		if signErr != nil {
			return signErr
		}
		signature = "0x" + hex.EncodeToString(sig)
		return nil
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

type EvmTypedDataSigner struct {
	typedDataJSON string
	digest        string
	err           error
}

func NewEvmTypedDataSigner(typedDataJSON string) *EvmTypedDataSigner {
	s := &EvmTypedDataSigner{typedDataJSON: typedDataJSON}
	digest, err := evm.HashTypedDataJSON([]byte(typedDataJSON))
	if err != nil {
		s.err = fmt.Errorf("invalid_input: %v", err)
		return s
	}
	s.digest = "0x" + hex.EncodeToString(digest)
	return s
}

func (s *EvmTypedDataSigner) Digest() string {
	return s.digest
}

func (s *EvmTypedDataSigner) Sign(privateKey []byte, expectedSigner string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	sig, err := evm.SignTypedDataJSON(privateKey, []byte(s.typedDataJSON), expectedSigner)
	WipeByte(privateKey)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(sig), nil
}

func (s *EvmTypedDataSigner) SignWithSession(handle string) (string, error) {
	return s.SignWithSessionExpecting(handle, "")
}

func (s *EvmTypedDataSigner) SignWithSessionExpecting(handle, expectedDigest string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if err := matchesExpectedDigest(s.digest, expectedDigest); err != nil {
		return "", err
	}
	var signature string
	err := session.WithKey(handle, session.ChainEVM, func(privateKey []byte, address string) error {
		sig, signErr := evm.SignTypedDataJSON(privateKey, []byte(s.typedDataJSON), address)
		if signErr != nil {
			return signErr
		}
		signature = "0x" + hex.EncodeToString(sig)
		return nil
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func matchesExpectedDigest(got, expected string) error {
	if expected == "" {
		return nil
	}
	if !strings.EqualFold(strings.TrimPrefix(got, "0x"), strings.TrimPrefix(expected, "0x")) {
		return fmt.Errorf("digest_mismatch: the preview does not describe what would be signed")
	}
	return nil
}
