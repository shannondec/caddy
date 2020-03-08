// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package caddypki

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"strings"
)

func pemDecodeSingleCert(pemDER []byte) (*x509.Certificate, error) {
	pemBlock, remaining := pem.Decode(pemDER)
	if pemBlock == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if len(remaining) > 0 {
		return nil, fmt.Errorf("input contained more than a single PEM block")
	}
	if pemBlock.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("expected PEM block type to be CERTIFICATE, but got '%s'", pemBlock.Type)
	}
	return x509.ParseCertificate(pemBlock.Bytes)
}

func pemEncodeCert(der []byte) ([]byte, error) {
	return pemEncode("CERTIFICATE", der)
}

// pemEncodePrivateKey marshals a EC or RSA private key into a PEM-encoded array of bytes.
// TODO: this is the same thing as in certmagic. Should we reuse that code somehow? It's unexported.
func pemEncodePrivateKey(key crypto.PrivateKey) ([]byte, error) {
	var pemType string
	var keyBytes []byte
	switch key := key.(type) {
	case *ecdsa.PrivateKey:
		var err error
		pemType = "EC"
		keyBytes, err = x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, err
		}
	case *rsa.PrivateKey:
		pemType = "RSA"
		keyBytes = x509.MarshalPKCS1PrivateKey(key)
	case *ed25519.PrivateKey:
		var err error
		pemType = "ED25519"
		keyBytes, err = x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported key type: %T", key)
	}
	return pemEncode(pemType+" PRIVATE KEY", keyBytes)
}

// pemDecodePrivateKey loads a PEM-encoded ECC/RSA private key from an array of bytes.
// Borrowed from Go standard library, to handle various private key and PEM block types.
// https://github.com/golang/go/blob/693748e9fa385f1e2c3b91ca9acbb6c0ad2d133d/src/crypto/tls/tls.go#L291-L308
// https://github.com/golang/go/blob/693748e9fa385f1e2c3b91ca9acbb6c0ad2d133d/src/crypto/tls/tls.go#L238)
// TODO: this is the same thing as in certmagic. Should we reuse that code somehow? It's unexported.
func pemDecodePrivateKey(keyPEMBytes []byte) (crypto.PrivateKey, error) {
	keyBlockDER, _ := pem.Decode(keyPEMBytes)

	if keyBlockDER.Type != "PRIVATE KEY" && !strings.HasSuffix(keyBlockDER.Type, " PRIVATE KEY") {
		return nil, fmt.Errorf("unknown PEM header %q", keyBlockDER.Type)
	}

	if key, err := x509.ParsePKCS1PrivateKey(keyBlockDER.Bytes); err == nil {
		return key, nil
	}

	if key, err := x509.ParsePKCS8PrivateKey(keyBlockDER.Bytes); err == nil {
		switch key := key.(type) {
		case *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey:
			return key, nil
		default:
			return nil, fmt.Errorf("found unknown private key type in PKCS#8 wrapping: %T", key)
		}
	}

	if key, err := x509.ParseECPrivateKey(keyBlockDER.Bytes); err == nil {
		return key, nil
	}

	return nil, fmt.Errorf("unknown private key type")
}

func pemEncode(blockType string, b []byte) ([]byte, error) {
	var buf bytes.Buffer
	err := pem.Encode(&buf, &pem.Block{Type: blockType, Bytes: b})
	return buf.Bytes(), err
}

func trusted(cert *x509.Certificate) bool {
	chains, err := cert.Verify(x509.VerifyOptions{})
	return len(chains) > 0 && err == nil
}

// KeyPair represents a public-private key pair, where the
// public key is also called a certificate.
type KeyPair struct {
	Certificate string `json:"certificate,omitempty"`
	PrivateKey  string `json:"private_key,omitempty"`
	Format      string `json:"format,omitempty"`
}

// Load loads the certificate and key.
func (kp KeyPair) Load() (*x509.Certificate, interface{}, error) {
	switch kp.Format {
	case "", "pem_file":
		certData, err := ioutil.ReadFile(kp.Certificate)
		if err != nil {
			return nil, nil, err
		}
		keyData, err := ioutil.ReadFile(kp.PrivateKey)
		if err != nil {
			return nil, nil, err
		}

		cert, err := pemDecodeSingleCert(certData)
		if err != nil {
			return nil, nil, err
		}
		key, err := pemDecodePrivateKey(keyData)
		if err != nil {
			return nil, nil, err
		}

		return cert, key, nil

	default:
		return nil, nil, fmt.Errorf("unsupported format: %s", kp.Format)
	}
}
