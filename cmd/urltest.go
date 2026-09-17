package main

import (
	"fmt"
	"github.com/asaskevich/govalidator"
)

func main() {
	uris := []string{
		"https://example.com/a.pdf",
		"data:application/pdf;base64,JVBERi0xLjQ=",
		"data:application/pdf;base64,JVBERi0xLjQ%3D",
	}
	for _, u := range uris {
		fmt.Printf("%-50s -> %v\n", u, govalidator.IsURL(u))
	}
}
