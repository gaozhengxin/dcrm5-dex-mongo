//Author: xingchang@fusion.org
package paillier

import (
	"github.com/fusion/go-fusion/common/math/random"
	"math/big"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	publicKey, privateKey := GenerateKeyPair(1024)
	t.Log(publicKey)
	t.Log(privateKey)
}

func TestEncrypt(t *testing.T) {
	publicKey, _ := GenerateKeyPair(1024)

	one := big.NewInt(1)

	//cipher, _ := publicKey.Encrypt(one)
	cipher, _,_ := publicKey.Encrypt(one)
	t.Log(cipher)
}

func TestDecrypt(t *testing.T) {
	publicKey, privateKey := GenerateKeyPair(1024)

	num := random.GetRandomIntFromZn(publicKey.N)
	t.Log(num)

	//cipher, _ := publicKey.Encrypt(num)
	cipher, _,_ := publicKey.Encrypt(num)

	m, _ := privateKey.Decrypt(cipher)
	t.Log(m)
}

func TestHomoAdd(t *testing.T) {
	publicKey, privateKey := GenerateKeyPair(1024)

	num1 := random.GetRandomIntFromZn(publicKey.N)
	num2 := random.GetRandomIntFromZn(publicKey.N)

	sum := new(big.Int).Add(num1, num2)
	sum = new(big.Int).Mod(sum, publicKey.N)
	t.Log(sum)

	//cOne, _ := publicKey.Encrypt(num1)
	//cTwo, _ := publicKey.Encrypt(num2)
	cOne, _,_ := publicKey.Encrypt(num1)
	cTwo, _,_ := publicKey.Encrypt(num2)

	cNew := publicKey.HomoAdd(cOne, cTwo)

	m, _ := privateKey.Decrypt(cNew)
	t.Log(m)
}

func TestHomoMul(t *testing.T) {
	publicKey, privateKey := GenerateKeyPair(1024)

	num1 := random.GetRandomIntFromZn(publicKey.N)
	num2 := random.GetRandomIntFromZn(publicKey.N)

	times := new(big.Int).Mul(num1, num2)
	times = new(big.Int).Mod(times, publicKey.N)
	t.Log(times)

	//cOne, _ := publicKey.Encrypt(num1)
	cOne, _,_ := publicKey.Encrypt(num1)
	cNew := publicKey.HomoMul(cOne, num2)

	m, _ := privateKey.Decrypt(cNew)
	t.Log(m)
}

func TestZkFactProve(t *testing.T) {
	_, privateKey := GenerateKeyPair(1024)

	zkFactProof := privateKey.ZkFactProve()

	t.Log(zkFactProof)
}

func TestZkFactVerify(t *testing.T) {
	publicKey, privateKey := GenerateKeyPair(1024)

	zkFactProof := privateKey.ZkFactProve()

	rlt := publicKey.ZkFactVerify(zkFactProof)

	t.Log(rlt)
}
