package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

func main() {
	if os.Args[1] == "split" {
		json.NewEncoder(os.Stdout).Encode([][]string{{"1", "100"}, {"101", "200"}})
		return
	}
	start, _ := strconv.Atoi(os.Args[2])
	end, _ := strconv.Atoi(os.Args[3])
	total := 0
	for n := start; n <= end; n++ {
		total += n
	}
	fmt.Println(total)
}
