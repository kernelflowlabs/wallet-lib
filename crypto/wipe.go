package crypto

func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
