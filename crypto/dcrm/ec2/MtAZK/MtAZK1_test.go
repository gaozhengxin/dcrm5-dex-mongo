package MtAZK

import (
	"github.com/fusion/go-fusion/common/math/random"
	"github.com/fusion/go-fusion/crypto/dcrm/ec2/paillier"
	s256 "github.com/fusion/go-fusion/crypto/secp256k1"
	"testing"
)

func TestMtAZK1Prove(t *testing.T) {
	pk, sk := paillier.GenerateKeyPair(2048)
	m := random.GetRandomIntFromZn(s256.S256().N)
	_, r, _ := pk.Encrypt(m)
	zkF := sk.ZkFactProve()

	mtAZK1Proof := MtAZK1Prove(m, r, pk, zkF)

	t.Log(mtAZK1Proof)
}

func TestMtAZK1Verify(t *testing.T) {
	pk, sk := paillier.GenerateKeyPair(2048)
	m := random.GetRandomIntFromZn(s256.S256().N)
	c, r, _ := pk.Encrypt(m)
	zkF := sk.ZkFactProve()

	mtAZK1Proof := MtAZK1Prove(m, r, pk, zkF)

	rlt := mtAZK1Proof.MtAZK1Verify(c, pk, zkF)

	t.Log(rlt)
}
