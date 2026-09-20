// Package awstest はローカル AWS 統合テスト向けの共通 helper を提供する。
package awstest

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	nanoIDAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	nanoIDLength   = 16
)

// NanoID は AWS の一時リソース名に使える英小文字・数字のランダム ID を返す。
func NanoID() string {
	id := make([]byte, nanoIDLength)
	limit := big.NewInt(int64(len(nanoIDAlphabet)))
	for i := range id {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			panic(fmt.Sprintf("awstest: generate nano id: %v", err))
		}
		id[i] = nanoIDAlphabet[index.Int64()]
	}
	return string(id)
}

// ResourceName は prefix に NanoID を付け、テスト間で衝突しないリソース名を返す。
func ResourceName(prefix string) string {
	return prefix + "-" + NanoID()
}
