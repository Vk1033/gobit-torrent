package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"unicode"
	// bencode "github.com/jackpal/bencode-go"
)

var _ = json.Marshal

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
		list := []any{}
		for i := 1; i < len(bencodedString); i++ {
			if bencodedString[i] == 'e' {
				break
			}
			value, err := decodeBencode(bencodedString[i:])
			if err != nil {
				return "", err
			}
			list = append(list, value)
			i += len(fmt.Sprintf("%v", value)) + 1

		}

		return list, nil
	} else if rune(bencodedString[0]) == 'd' {
		dict := map[string]any{}
		for i := 1; i < len(bencodedString); i++ {
			if bencodedString[i] == 'e' {
				break
			}
			key, err := decodeBencode(bencodedString[i:])
			if err != nil {
				return "", err
			}
			i += len(fmt.Sprintf("%v", key)) + 2
			value, err := decodeBencode(bencodedString[i:])
			if err != nil {
				return "", err
			}
			dict[fmt.Sprintf("%v", key)] = value
			i += len(fmt.Sprintf("%v", value)) + 1
		}
		return dict, nil
	}

	return nil, fmt.Errorf("UNSUPPORTED TYPE")
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
