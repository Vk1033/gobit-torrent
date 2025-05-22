package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"unicode"
	// bencode "github.com/jackpal/bencode-go"
)

var _ = json.Marshal

func decodeBencode(bencodedString string) (any, int, error) {
	if unicode.IsDigit(rune(bencodedString[0])) {
		var firstColonIndex int
		var i int

		for i = 0; i < len(bencodedString); i++ {
			if bencodedString[i] == ':' {
				firstColonIndex = i
				break
			}
		}

		lengthStr := bencodedString[:firstColonIndex]

		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return "", 0, err
		}
		return bencodedString[firstColonIndex+1 : firstColonIndex+1+length], i + length, nil
	} else if rune(bencodedString[0]) == 'i' {
		var endIndex int
		var i int
		for i = 1; i < len(bencodedString); i++ {
			if bencodedString[i] == 'e' {
				endIndex = i
				break
			}
		}
		if endIndex == 0 {
			return "", 0, fmt.Errorf("INVALID BENCODED INTEGER")
		}
		intValue, err := strconv.Atoi(bencodedString[1:endIndex])
		if err != nil {
			return "", 0, err
		}
		return intValue, i, nil
	} else if rune(bencodedString[0]) == 'l' {
		list := []any{}
		var i int
		for i = 1; i < len(bencodedString); {
			if bencodedString[i] == 'e' {
				break
			}
			value, valueLen, err := decodeBencode(bencodedString[i:])
			if err != nil {
				return "", 0, err
			}
			list = append(list, value)
			i += valueLen + 1

		}

		return list, i, nil
	} else if rune(bencodedString[0]) == 'd' {
		dict := map[string]any{}
		var i int
		for i = 1; i < len(bencodedString); {
			if bencodedString[i] == 'e' {
				break
			}
			key, keyLen, err := decodeBencode(bencodedString[i:])
			if err != nil {
				return "", 0, err
			}
			i += keyLen + 1
			value, valueLen, err := decodeBencode(bencodedString[i:])
			if err != nil {
				return "", 0, err
			}
			dict[fmt.Sprintf("%v", key)] = value
			i += valueLen + 1
		}
		return dict, i, nil
	}

	return nil, 0, fmt.Errorf("UNSUPPORTED TYPE")
}

func bencodeEncode(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%d:%s", len(v), v)
	case int:
		return fmt.Sprintf("i%de", v)
	case []any:
		result := "l"
		for _, item := range v {
			result += bencodeEncode(item)
		}
		result += "e"
		return result
	case map[string]any:
		result := "d"
		// keys must be sorted
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			result += bencodeEncode(k)
			result += bencodeEncode(v[k])
		}
		result += "e"
		return result
	default:
		panic(fmt.Sprintf("Unsupported type: %T", v))
	}
}

func main() {
	command := os.Args[1]

	if command == "decode" {
		bencodedValue := os.Args[2]

		decoded, _, err := decodeBencode(bencodedValue)
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else if command == "info" {
		fileName := os.Args[2]
		data, err := os.ReadFile(fileName)
		if err != nil {
			panic(err)
		}
		bencodedString := string(data)
		decoded, _, err := decodeBencode(bencodedString)
		if err != nil {
			fmt.Println(err)
			return
		}
		dict, ok := decoded.(map[string]any)
		if !ok {
			fmt.Println("Invalid bencoded data")
			return
		}
		info, ok := dict["info"].(map[string]any)
		if !ok {
			fmt.Println("Invalid bencoded data")
			return
		}
		encodedInfo := bencodeEncode(info)
		hash := sha1.Sum([]byte(encodedInfo))

		fmt.Printf("Tracker URL: %s\n", dict["announce"])
		fmt.Printf("Length: %d\n", info["length"])
		fmt.Printf("Info Hash: %x\n", hash)

	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
