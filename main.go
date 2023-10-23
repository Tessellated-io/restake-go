package main

import (
	"fmt"
	"regexp"
)

// TODO: Reap this file

func main() {
	str1 := "provided fee < minimum global fee (96826250658418aevmos < 7746100000000000aevmos). Please increase the gas price.: insufficient fee"
	extract(str1)

	str2 := "provided fee < minimum global fee (96826250658418aevmos < 7746100000000000aevmos). Please increase the gas price.: insufficient fee"
	extract(str2)
}

func extract(str string) {
	// Regular expression to match the desired number
	pattern := `(\d+)\w+\)\. Please increase`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(str)
	if len(matches) > 1 {
		fmt.Println(matches[1]) // This will print 7746100000000000
	} else {
		fmt.Println("No match found")
	}
}
