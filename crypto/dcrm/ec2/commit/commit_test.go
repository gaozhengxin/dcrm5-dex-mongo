//Author: xingchang@fusion.org
package commit

import (
	"math/big"
	"testing"
)

func TestCommit(t *testing.T) {
	zero := big.NewInt(0)
	one := big.NewInt(1)

	commitment := new(Commitment).Commit(zero, one)
	t.Log(commitment.C)
	t.Log(commitment.D)
}

func TestVerify(t *testing.T) {
	zero := big.NewInt(0)
	one := big.NewInt(1)

	commitment := new(Commitment).Commit(zero, one)
	t.Log(commitment.C)
	t.Log(commitment.D)

	pass := commitment.Verify()
	t.Log(pass)
}

func TestDeCommit(t *testing.T) {
	zero := big.NewInt(0)
	one := big.NewInt(1)

	commitment := new(Commitment).Commit(zero, one)
	t.Log(commitment.C)
	t.Log(commitment.D)

	pass, secrets := commitment.DeCommit()
	t.Log(pass)
	t.Log(secrets)
}
