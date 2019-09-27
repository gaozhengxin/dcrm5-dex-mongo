//Author: xingchang@fusion.org
package vss

import (
	"github.com/ethereum/go-ethereum/common/math/random"
	s256 "github.com/ethereum/go-ethereum/crypto/secp256k1"
	"math/big"
	"testing"
)

func TestVss(t *testing.T) {
	threshold := 2
	total := 3
	t.Log(threshold)
	t.Log(total)

	secret := random.GetRandomIntFromZn(s256.S256().N)
	t.Log(secret)

	ids := make([]*big.Int, 0)
	for i := 0; i < total; i++ {
		ids = append(ids, random.GetRandomIntFromZn(s256.S256().N))
	}
	t.Log(ids)

	polyG, poly, shares, err := Vss(secret, ids, threshold, total)

	t.Log(err)
	t.Log(polyG)
	t.Log(poly)
	t.Log(shares)
}

func TestVerify(t *testing.T) {
	threshold := 2
	total := 3
	secret := random.GetRandomIntFromZn(s256.S256().N)

	ids := make([]*big.Int, 0)
	for i := 0; i < total; i++ {
		ids = append(ids, random.GetRandomIntFromZn(s256.S256().N))
	}

	polyG, _, shares, _ := Vss(secret, ids, threshold, total)

	for i := 0; i < total; i++ {
		t.Log(shares[i].Verify(polyG))
	}
}

func TestCombine(t *testing.T) {
	threshold := 2
	total := 3
	secret := random.GetRandomIntFromZn(s256.S256().N)
	t.Log(secret)

	ids := make([]*big.Int, 0)
	for i := 0; i < total; i++ {
		ids = append(ids, random.GetRandomIntFromZn(s256.S256().N))
	}

	_, _, shares, _ := Vss(secret, ids, threshold, total)

	secret2, err2 := Combine(shares[:threshold])
	t.Log(err2)
	t.Log(secret2)

	secret3, err3 := Combine(shares[:threshold-1])
	t.Log(err3)
	t.Log(secret3)

	secret4, err4 := Combine(shares[:total])
	t.Log(err4)
	t.Log(secret4)
}
