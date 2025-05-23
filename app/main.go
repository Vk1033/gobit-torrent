package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"time"
	// bencode "github.com/jackpal/bencode-go"
)

var _ = json.Marshal

const BlockSize = 16 * 1024 // 16 KiB

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
		fmt.Printf("Piece Length: %d\n", info["piece length"])
		fmt.Printf("Piece Hashes:\n")

		piecesStr := info["pieces"].(string)
		bytesPieces := []byte(piecesStr)
		for i := 0; i < len(bytesPieces); i += 20 {
			pieceHash := bytesPieces[i : i+20]
			fmt.Println(hex.EncodeToString(pieceHash))
		}

	} else if command == "peers" {
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

		peerList, err := getPeers(dict["announce"].(string), info)
		if err != nil {
			fmt.Println("Error getting peers:", err)
			return
		}
		for _, peer := range peerList {
			fmt.Println(peer)
		}

	} else if command == "handshake" {
		fileName := os.Args[2]
		address := os.Args[3]

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
		conn, err := net.DialTimeout("tcp", address, 30*time.Second)
		if err != nil {
			fmt.Println("Error connecting to peer:", err)
			return
		}
		defer conn.Close()
		encodedInfo := bencodeEncode(info)
		hash := sha1.Sum([]byte(encodedInfo))
		infoHash := fmt.Sprintf("%s", hash)

		// Perform handshake
		err = doHandShake(conn, infoHash)
		if err != nil {
			fmt.Println("Error during handshake:", err)
			return
		}
		// Read the handshake response
		res, err := readHandShake(conn)
		peerID := res[48:68]
		if err != nil {
			fmt.Println("Error reading handshake response:", err)
			return
		}

		fmt.Printf("Peer ID: %x\n", peerID)

	} else if command == "download_piece" {
		outputFile := os.Args[3]
		fileName := os.Args[4]
		pieceIndex, err := strconv.Atoi(os.Args[5])
		if err != nil {
			fmt.Println("Error converting piece index:", err)
			return
		}
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
		peerList, err := getPeers(dict["announce"].(string), info)
		if err != nil {
			fmt.Println("Error getting peers:", err)
			return
		}
		// Connect to the first peer
		conn, err := net.DialTimeout("tcp", peerList[0], 30*time.Second)
		if err != nil {
			fmt.Println("Error connecting to peer:", err)
			return
		}
		defer conn.Close()
		encodedInfo := bencodeEncode(info)
		hash := sha1.Sum([]byte(encodedInfo))
		infoHash := fmt.Sprintf("%s", hash)
		err = doHandShake(conn, infoHash)
		if err != nil {
			fmt.Println("Error during handshake:", err)
			return
		}
		_, err = readHandShake(conn)
		if err != nil {
			fmt.Println("Error reading handshake response:", err)
			return
		}

		_, err = readBitfield(conn)
		if err != nil {
			fmt.Println("Error reading bitfield:", err)
			return
		}
		err = sendInterested(conn)
		if err != nil {
			fmt.Println("Error sending interested message:", err)
			return
		}
		err = readUnchoke(conn)
		if err != nil {
			fmt.Println("Error reading unchoke message:", err)
			return
		}
		pieceLength := int(info["piece length"].(int))
		// right after you fetch pieceLength from info
		totalLength := int(info["length"].(int))
		numPieces := (totalLength + pieceLength - 1) / pieceLength

		if pieceIndex == numPieces-1 {
			remaining := totalLength % pieceLength
			if remaining != 0 {
				pieceLength = remaining
			}
		}

		begin := 0

		for begin < pieceLength {
			blockLen := BlockSize
			if begin+blockLen > pieceLength {
				blockLen = pieceLength - begin
			}
			err := sendRequest(conn, pieceIndex, begin, blockLen)
			if err != nil {
				fmt.Printf("Failed to send request: %v\n", err)
				return
			}
			begin += blockLen
		}

		pieceBuffer := make([]byte, pieceLength)
		blocksReceived := 0

		for blocksReceived < pieceLength {
			// Read message length
			lengthBuf := make([]byte, 4)
			_, err := io.ReadFull(conn, lengthBuf)
			if err != nil {
				fmt.Println("Failed to read message length:", err)
				return
			}
			length := binary.BigEndian.Uint32(lengthBuf)

			// Read full message
			payload := make([]byte, length)
			_, err = io.ReadFull(conn, payload)
			if err != nil {
				fmt.Println("Failed to read message payload:", err)
				return
			}

			if payload[0] != 7 {
				// Not a piece message, skip or handle
				continue
			}

			index := binary.BigEndian.Uint32(payload[1:5])
			begin := binary.BigEndian.Uint32(payload[5:9])
			block := payload[9:]

			if int(index) != pieceIndex {
				fmt.Println("Received block for wrong piece, skipping")
				continue
			}

			copy(pieceBuffer[begin:], block)
			blocksReceived += len(block)

		}

		err = os.WriteFile(outputFile, pieceBuffer, 0644)
		if err != nil {
			fmt.Println("Failed to save piece:", err)
			return
		}
		fmt.Println("Piece downloaded and saved successfully.")

		// validate piece
		calculatedHash := sha1.Sum(pieceBuffer)
		fmt.Printf("%x\n", calculatedHash)

	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
