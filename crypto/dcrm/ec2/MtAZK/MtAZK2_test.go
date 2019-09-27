package MtAZK

import (
	"github.com/fusion/go-fusion/common/math/random"
	"github.com/fusion/go-fusion/crypto/dcrm/ec2/paillier"
	s256 "github.com/fusion/go-fusion/crypto/secp256k1"
	"testing"
)

func TestMtAZK2Prove(t *testing.T) {
	pk, sk := paillier.GenerateKeyPair(2048)

	m := random.GetRandomIntFromZn(s256.S256().N)

	x := random.GetRandomIntFromZn(s256.S256().N)
	y := random.GetRandomIntFromZn(s256.S256().N)

	c1, _, _ := pk.Encrypt(m)
	c1x := pk.HomoMul(c1, x)

	c2, r, _ := pk.Encrypt(y)
	c2 = pk.HomoAdd(c1x, c2)

	zkF := sk.ZkFactProve()

	mtAZK2Proof := MtAZK2Prove(x, y, r, c1, pk, zkF)

	t.Log(mtAZK2Proof)
}

func TestMtAZK2Verify(t *testing.T) {
	pk, sk := paillier.GenerateKeyPair(2048)

	m := random.GetRandomIntFromZn(s256.S256().N)

	x := random.GetRandomIntFromZn(s256.S256().N)
	y := random.GetRandomIntFromZn(s256.S256().N)

	c1, _, _ := pk.Encrypt(m)
	c1x := pk.HomoMul(c1, x)

	c2, r, _ := pk.Encrypt(y)
	c2 = pk.HomoAdd(c1x, c2)

	zkF := sk.ZkFactProve()

	mtAZK2Proof := MtAZK2Prove(x, y, r, c1, pk, zkF)

	rlt := mtAZK2Proof.MtAZK2Verify(c1, c2, pk, zkF)

	t.Log(rlt)
}
