package debug

import (
	"log"
	"os"
)

func Enabled() bool {
	return os.Getenv("APP_ENV") == "development" || os.Getenv("GIN_MODE") == "debug"
}

func Logf(format string, args ...interface{}) {
	if Enabled() {
		log.Printf("[DEBUG] "+format, args...)
	}
}
