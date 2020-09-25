package main

import (
	"fmt"
	"github.io/gnu3ra/localstack/stack"
)

func main() {
	fmt.Println("empty")

	_, err := stack.NewDockerStack(&stack.DockerStackConfig{
		Name: "test",
	})

	if err != nil {
		fmt.Printf("Error createing dockerstack: %v", err)
	}

	fmt.Println("created blank dockerstack")
}
