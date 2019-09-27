package main

import (
	"encoding/hex"
	"crypto/ecdsa"
	ethcrypto "github.com/fusion/go-fusion/crypto"
)

func SignTransaction(hash []string, privateKey *ecdsa.PrivateKey) (rsv []string, err error) {
	for i, _ := range hash {
		hashBytes, err := hex.DecodeString(hash[i])
		if err != nil {
			return nil, err
		}
		/*r, s, err := ecdsa.Sign(rand.Reader, privateKey.(*ecdsa.PrivateKey), hashBytes)
		if err != nil {
			return
		}
		fmt.Printf("r: %v\ns: %v\n\n", r, s)
		rx := fmt.Sprintf("%X", r)
		sx := fmt.Sprintf("%X", s)
		rsv = append(rsv, rx + sx + "00")*/
		rsvBytes, err := ethcrypto.Sign(hashBytes, privateKey)
		if err != nil {
			return nil, err
		}
		rsv = append(rsv, hex.EncodeToString(rsvBytes))
	}
	return
}
