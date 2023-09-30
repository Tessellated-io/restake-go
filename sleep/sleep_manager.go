package sleep

import (
	"fmt"
	"time"
)

// How long to sleep for
const sleepDurationMilliseconds = 5000

func Sleep() {
	fmt.Printf("	> Sleeping for %d seconds\n", sleepDurationMilliseconds)
	time.Sleep(sleepDurationMilliseconds * time.Millisecond)
}
