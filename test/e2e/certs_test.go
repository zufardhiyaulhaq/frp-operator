//go:build e2e

package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// Certs are PEM blocks generated for one suite run; nothing is stored in the repo.
type Certs struct {
	CA         []byte // signs the frps server cert and the frpc client cert
	ServerCert []byte // frps-main, frps-mtls and wss-terminator (one cert, all three DNS names)
	ServerKey  []byte
	ClientCert []byte // frpc client cert for mTLS
	ClientKey  []byte
	OtherCA    []byte // an unrelated CA, for "a wrong CA is rejected"
}

func newCerts(serverNames []string) (*Certs, error) {
	ca, caKey, caPEM, err := newCA("frp-operator-e2e CA")
	if err != nil {
		return nil, err
	}
	serverCert, serverKey, err := newLeaf(ca, caKey, "frps", serverNames, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return nil, err
	}
	clientCert, clientKey, err := newLeaf(ca, caKey, "frpc", nil, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return nil, err
	}
	_, _, otherPEM, err := newCA("unrelated CA")
	if err != nil {
		return nil, err
	}
	return &Certs{CA: caPEM, ServerCert: serverCert, ServerKey: serverKey, ClientCert: clientCert, ClientKey: clientKey, OtherCA: otherPEM}, nil
}

func newCA(name string) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // reused for the life of the cluster
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, err
	}
	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

func newLeaf(ca *x509.Certificate, caKey *ecdsa.PrivateKey, name string, dnsNames []string, usage x509.ExtKeyUsage) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		panic(err)
	}
	return n
}
