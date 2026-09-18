package utils

import (
	"fmt"
	"math/rand"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// EnvSlice converts an env var map into "K=V" form, as expected by exec-style APIs.
func EnvSlice(envs map[string]string) []string {
	env := make([]string, 0, len(envs))
	for k, v := range envs {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func FormatBps(bps int64) string {
	const unit = 1000
	if bps < unit {
		return fmt.Sprintf("%dbps", bps)
	}
	div, exp := int64(unit), 0
	for n := bps / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cbps", float64(bps)/float64(div), "KMGTPE"[exp])
}
