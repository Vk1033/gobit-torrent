package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"unicode"
	// bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

// Ensures gofmt doesn't remove the "os" encoding/json import (feel free to remove this!)
var _ = json.Marshal

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345
func decodeBencode(bencodedString string) (interface{}, error) {
	if unicode.IsDigit(rune(bencodedString[0])) {
		var firstColonIndex int

		for i := 0; i < len(bencodedString); i++ {
			if bencodedString[i] == ':' {
				firstColonIndex = i
				break
			}
		}

		lengthStr := bencodedString[:firstColonIndex]

		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return "", err
		}

		return bencodedString[firstColonIndex+1 : firstColonIndex+1+length], nil
	} else if rune(bencodedString[0]) == 'i' {
		var endIndex int
		for i := 1; i < len(bencodedString); i++ {
			if bencodedString[i] == 'e' {
				endIndex = i
				break
			}
		}
		if endIndex == 0 {
			return "", fmt.Errorf("INVALID BENCODED INTEGER")
		}
		intValue, err := strconv.Atoi(bencodedString[1:endIndex])
		if err != nil {
			return "", err
		}
		return intValue, nil
	} else if rune(bencodedString[0]) == 'l' {
		var list []any
		var endIndex int
		for i := 1; i < len(bencodedString); i++ {
			if bencodedString[i] == 'e' {
				endIndex = i
				break
			}
			fmt.Printf("Bencoded item: %s\n", bencodedString[i:])
			value, err := decodeBencode(bencodedString[i:])
			if err != nil {
				return "", err
			}
			fmt.Printf("Decoded value: %v\n", value)
			list = append(list, value)
			i += len(fmt.Sprintf("%v", value)) + 1

		}
		fmt.Printf("Bencoded list: %s\n", bencodedString)
		if endIndex == 0 {
			return "", fmt.Errorf("INVALID BENCODED LIST")
		}
		return list, nil
	}

	return "", fmt.Errorf("UNSUPPORTED TYPE")
}

func main() {
	command := os.Args[1]

	if command == "decode" {
		bencodedValue := os.Args[2]

		decoded, err := decodeBencode(bencodedValue)
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
