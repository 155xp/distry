package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("use: distry coordinator | worker <url> | submit <url> <file.go>")
		return
	}

	switch os.Args[1] {
	case "coordinator":
		startCoordinator()
	case "worker":
		if len(os.Args) != 3 {
			fmt.Println("use: distry worker <coordinator-url>")
			return
		}
		startWorker(os.Args[2])
	case "submit":
		if len(os.Args) != 4 {
			fmt.Println("use: distry submit <coordinator-url> <file.go>")
			return
		}
		if err := submit(os.Args[2], os.Args[3]); err != nil {
			fmt.Println("submit failed:", err)
		}
	default:
		fmt.Println("unknown command")
	}
}
