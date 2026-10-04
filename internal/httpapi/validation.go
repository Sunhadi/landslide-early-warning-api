package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// decodeBody membaca body JSON. Mengembalikan true bila berhasil; bila gagal,
// respons error sudah ditulis (400 untuk JSON tidak valid, 413 bila terlalu besar).
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				"Body permintaan melebihi batas ukuran")
			return false
		}
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Body JSON tidak valid")
		return false
	}
	return true
}

// phonePattern menerima format nomor Indonesia: +62..., 62..., atau 08...
var phonePattern = regexp.MustCompile(`^(\+62|62|0)8[1-9][0-9]{6,11}$`)

// normalizePhone membersihkan nomor dari spasi, tanda hubung, dan tanda kurung.
func normalizePhone(s string) string {
	repl := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
	return repl.Replace(strings.TrimSpace(s))
}

// validPhone memeriksa apakah nomor telepon Indonesia valid.
func validPhone(s string) bool {
	return phonePattern.MatchString(normalizePhone(s))
}

// validLatLon memeriksa rentang koordinat.
func validLatLon(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// validText memeriksa panjang teks (dalam rune) dan menolak teks kosong.
func validText(s string, maxLen int) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	return utf8.RuneCountInString(s) <= maxLen
}

// oneOf memeriksa apakah nilai termasuk dalam daftar yang diizinkan.
func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
