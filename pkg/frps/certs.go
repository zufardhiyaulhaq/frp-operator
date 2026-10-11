package frps

import (
	"crypto/tls"
	"crypto/x509"
)

// ServerNames checks that certPEM and keyPEM are a matching pair and returns the certificate's
// DNS names, which a Client puts in tls.serverName.
func ServerNames(certPEM, keyPEM []byte) ([]string, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	return leaf.DNSNames, nil
}
