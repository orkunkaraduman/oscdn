package apps

import (
	"fmt"
	"math/rand"
)

func generateRequestHash() string {
	return fmt.Sprintf("%08x", rand.Int63())
}
