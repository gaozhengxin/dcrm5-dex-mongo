package utils

import (
	"strconv"
	"strings"
)

func ParseQuantity(quantity string) string {
	n := strings.Split(quantity, " ")[0]
	num, err := strconv.ParseFloat(n, 64)
	if err != nil {
		return "0"
	}
	num2 := num * 10000
	return strconv.FormatFloat(num2, 'f', -1, 64)
}
