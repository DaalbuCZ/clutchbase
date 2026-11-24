package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

func ValidateWhitelist(ids []string) error {
	if len(ids) > 256 {
		return errors.New("use at most 256 SteamID64 values")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		value, err := strconv.ParseUint(id, 10, 64)
		// Public individual Steam accounts have a 32-bit nonzero account ID.
		if err != nil || len(id) != 17 || strconv.FormatUint(value, 10) != id || value <= 76561197960265728 || value > 76561202255233023 || seen[id] {
			return errors.New("use unique public individual SteamID64 values, one per line")
		}
		seen[id] = true
	}
	return nil
}

// The empty roster explicitly disables the whitelist while retaining the password.
func WhitelistHash(ids []string) string {
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n") + "\n"))
	return hex.EncodeToString(sum[:])
}
