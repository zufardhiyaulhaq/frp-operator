package frps

import (
	"reflect"
	"testing"

	"github.com/zufardhiyaulhaq/frp-operator/pkg/frps/frpstest"
)

func TestServerNames_ReturnsDNSNames(t *testing.T) {
	crt, key, err := frpstest.SelfSigned("frps.example.com", "frps.internal")
	if err != nil {
		t.Fatal(err)
	}
	names, err := ServerNames(crt, key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"frps.example.com", "frps.internal"}) {
		t.Errorf("ServerNames = %v", names)
	}
}

func TestServerNames_RejectsAKeyFromAnotherCertificate(t *testing.T) {
	crt, _, err := frpstest.SelfSigned("a.example.com")
	if err != nil {
		t.Fatal(err)
	}
	_, otherKey, err := frpstest.SelfSigned("b.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ServerNames(crt, otherKey); err == nil {
		t.Fatal("ServerNames accepted a key that does not match the certificate")
	}
}

func TestServerNames_RejectsGarbage(t *testing.T) {
	if _, err := ServerNames([]byte("not a certificate"), []byte("not a key")); err == nil {
		t.Fatal("ServerNames accepted garbage")
	}
}
