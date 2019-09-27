package utils

import "regexp"

func IsUserKey (memo string) bool {
	if ok, _ := regexp.Match("^d[1-5a-z]{32,33}$", []byte(memo)); ok {
		return true
	}
	return false
}
