package schnorrZK

import (
	"github.com/fusion/go-fusion/common/math/random"
	s256 "github.com/fusion/go-fusion/crypto/secp256k1"
	"math/big"
	"testing"
)

func TestZkUProve(t *testing.T) {
	u := random.GetRandomIntFromZn(s256.S256().N)
	zkUProof := ZkUProve(u)

	t.Log(zkUProof)
}

func TestZkUVerify(t *testing.T) {
	u := random.GetRandomIntFromZn(s256.S256().N)

	uGx, uGy := s256.S256().ScalarBaseMult(u.Bytes())
	uG := []*big.Int{uGx, uGy}

	zkUProof := ZkUProve(u)

	rlt := ZkUVerify(uG, zkUProof)

	t.Log(rlt)
}
