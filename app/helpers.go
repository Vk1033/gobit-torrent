package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"unicode"
)

type PieceTask struct {
	Index int
	Hash  [20]byte
	Size  int
}

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

func doHandShake(conn net.Conn, infoHash []byte) error {
	handShake := make([]byte, 68)
	handShake[0] = 19
	copy(handShake[1:], "BitTorrent protocol")

	copy(handShake[28:], infoHash)
	copy(handShake[48:], "-AZ2060-123456789012")
	handShake[68-1] = 0
	_, err := conn.Write(handShake)
	if err != nil {
		return err
	}
	return nil
}
func doMagnetHandShake(conn net.Conn, infoHash []byte) error {
	handShake := make([]byte, 68)
	handShake[0] = 19
	copy(handShake[1:], "BitTorrent protocol")
	// extension support
	handShake[25] = 16
	copy(handShake[28:], infoHash)
	copy(handShake[48:], "-AZ2060-123456789012")
	handShake[68-1] = 0
	_, err := conn.Write(handShake)
	if err != nil {
		return err
	}
	return nil
}

func readHandShake(conn net.Conn) ([]byte, error) {
	response := make([]byte, 68)
	_, err := io.ReadFull(conn, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func getPeers(trackerURL string, info map[string]any) ([]string, error) {
	client := &http.Client{}

	req, err := http.NewRequest(http.MethodGet, trackerURL, nil)
	if err != nil {
		return nil, err
	}
	encodedInfo := bencodeEncode(info)
	hash := sha1.Sum([]byte(encodedInfo))
	infoHash := fmt.Sprintf("%s", hash)
	length := info["length"].(int)

	peer_id := "-AZ2060-123456789012"
	url := req.URL.Query()
	url.Add("info_hash", infoHash)
	url.Add("peer_id", peer_id)
	url.Add("port", "6881")
	url.Add("uploaded", "0")
	url.Add("downloaded", "0")
	url.Add("left", strconv.Itoa(length))
	url.Add("compact", "1")
	req.URL.RawQuery = url.Encode()
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		fmt.Println("Error: ", res.Status)
		return nil, fmt.Errorf("tracker response error: %s", res.Status)
	}
	bencodedData, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return nil, err
	}
	decoded, _, err := decodeBencode(string(bencodedData))
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	dict, ok := decoded.(map[string]any)
	if !ok {
		fmt.Println("Invalid bencoded data")
		return nil, fmt.Errorf("invalid bencoded data")
	}
	peers, ok := dict["peers"].(string)
	if !ok {
		fmt.Println("Invalid bencoded data")
		return nil, fmt.Errorf("invalid bencoded data")
	}
	peersBytes := []byte(peers)
	if len(peersBytes)%6 != 0 {
		fmt.Println("Invalid peers data")
		return nil, fmt.Errorf("invalid peers data")
	}
	var peerList []string
	for i := 0; i < len(peersBytes); i += 6 {
		ip := fmt.Sprintf("%d.%d.%d.%d", peersBytes[i], peersBytes[i+1], peersBytes[i+2], peersBytes[i+3])
		port := (int(peersBytes[i+4]) << 8) + int(peersBytes[i+5])
		peerList = append(peerList, fmt.Sprintf("%s:%d", ip, port))
	}
	return peerList, nil
}

func readBitfield(conn net.Conn) ([]byte, error) {
	// Read 4-byte message length
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(conn, lengthBuf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// Read 1-byte message ID
	messageID := make([]byte, 1)
	_, err = io.ReadFull(conn, messageID)
	if err != nil {
		return nil, err
	}

	// Read payload
	payloadLen := int(length) - 1
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		_, err = io.ReadFull(conn, payload)
		if err != nil {
			return nil, err
		}
	}

	return payload, nil
}

func sendInterested(conn net.Conn) error {
	// Create interested message
	message := []byte{0, 0, 0, 1, 2} // length = 1 + 1 (message ID)
	_, err := conn.Write(message)
	if err != nil {
		return err
	}
	return nil
}

func readUnchoke(conn net.Conn) error {
	// Read 4-byte message length
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(conn, lengthBuf)
	if err != nil {
		return err
	}

	// Read 1-byte message ID
	messageID := make([]byte, 1)
	_, err = io.ReadFull(conn, messageID)
	if err != nil {
		return err
	}

	return nil
}

func sendRequest(conn net.Conn, index int, begin int, length int) error {
	msg := make([]byte, 17)
	msg[0] = 0  // Length prefix part 1
	msg[1] = 0  // Length prefix part 2
	msg[2] = 0  // Length prefix part 3
	msg[3] = 13 // Message length = 13
	msg[4] = 6  // Message ID = 6 (request)

	// Write the payload
	copy(msg[5:], intToBytes(index))
	copy(msg[9:], intToBytes(begin))
	copy(msg[13:], intToBytes(length))

	_, err := conn.Write(msg)
	return err
}

func intToBytes(n int) []byte {
	return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
}

func checkIntegrity(pieceBuffer []byte, pieceIndex int, info map[string]any) bool {
	piecesStr := info["pieces"].(string)
	bytesPieces := []byte(piecesStr)
	pieceHash := bytesPieces[pieceIndex*20 : (pieceIndex+1)*20]
	calculatedHash := sha1.Sum(pieceBuffer)

	return hex.EncodeToString(calculatedHash[:]) == hex.EncodeToString(pieceHash)
}

func handlePeer(addr string, tasks chan PieceTask, buffer [][]byte, info map[string]any) error {
	infoHash := sha1.Sum([]byte(bencodeEncode(info)))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	err = doHandShake(conn, infoHash[:])
	if err != nil {
		return err
	}
	readHandShake(conn)
	readBitfield(conn)
	sendInterested(conn)
	readUnchoke(conn)

	for task := range tasks {
		piece, err := downloadPiece(conn, task.Index, info)
		if err != nil {
			fmt.Printf("Failed to download piece %d: %v\n", task.Index, err)
			tasks <- task // requeue
			continue
		}
		check := checkIntegrity(piece, task.Index, info)
		if !check {
			tasks <- task // requeue
			continue
		}
		buffer[task.Index] = piece
	}
	return nil
}

func downloadPiece(conn net.Conn, pieceIndex int, info map[string]any) ([]byte, error) {
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
		fmt.Printf("Requesting piece %d, block starting at %d, length %d\n", pieceIndex, begin, blockLen)
		err := sendRequest(conn, pieceIndex, begin, blockLen)
		if err != nil {
			fmt.Printf("Failed to send request: %v\n", err)
			return nil, err
		}
		begin += blockLen
	}

	pieceBuffer := make([]byte, pieceLength)
	blocksReceived := 0

	for blocksReceived < pieceLength {
		fmt.Printf("Waiting for piece %d, blocks received: %d\n", pieceIndex, blocksReceived)
		// Read message length
		lengthBuf := make([]byte, 4)
		_, err := io.ReadFull(conn, lengthBuf)
		if err != nil {
			fmt.Println("Failed to read message length:", err)
			return nil, err
		}
		length := binary.BigEndian.Uint32(lengthBuf)

		// Read full message
		payload := make([]byte, length)
		_, err = io.ReadFull(conn, payload)
		if err != nil {
			fmt.Println("Failed to read message payload:", err)
			return nil, err
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
	return pieceBuffer, nil
}

func getPeersFromMagnet(trackerURL string, infoHash []byte) ([]string, error) {
	// Parse the magnet link to extract the info hash
	// In this case, we assume the info hash is already provided as a parameter
	// You can use a library like "github.com/zeebo/bencode" to decode the magnet link
	// For simplicity, we will just use the provided info hash directly
	// Create a new HTTP client

	client := &http.Client{}
	// Create a new HTTP GET request
	req, err := http.NewRequest(http.MethodGet, trackerURL, nil)
	if err != nil {
		return nil, err
	}
	// Set the query parameters for the tracker request
	url := req.URL.Query()
	url.Add("info_hash", string(infoHash[:]))
	url.Add("peer_id", "-AZ2060-123456789012")
	url.Add("port", "6881")
	url.Add("uploaded", "0")
	url.Add("downloaded", "0")
	url.Add("left", "999")
	url.Add("compact", "1")
	req.URL.RawQuery = url.Encode()
	// Send the request to the tracker
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	// Check if the response status is OK
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker response error: %s", res.Status)
	}
	// Read the response body
	bencodedData, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	// Decode the bencoded data
	decoded, _, err := decodeBencode(string(bencodedData))
	if err != nil {
		return nil, err
	}
	// Check if the decoded data is a dictionary
	dict, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid bencoded data")
	}
	// Extract the peers from the dictionary
	peers, ok := dict["peers"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid bencoded data")
	}
	// Convert the peers string to a byte slice
	peersBytes := []byte(peers)
	// Check if the length of the peers byte slice is a multiple of 6
	if len(peersBytes)%6 != 0 {
		return nil, fmt.Errorf("invalid peers data")
	}
	// Create a slice to hold the peer addresses
	var peerList []string
	// Iterate over the peers byte slice in chunks of 6 bytes
	for i := 0; i < len(peersBytes); i += 6 {
		// Extract the IP address from the first 4 bytes
		ip := fmt.Sprintf("%d.%d.%d.%d", peersBytes[i], peersBytes[i+1], peersBytes[i+2], peersBytes[i+3])
		// Extract the port number from the last 2 bytes
		port := (int(peersBytes[i+4]) << 8) + int(peersBytes[i+5])
		// Append the peer address to the list
		peerList = append(peerList, fmt.Sprintf("%s:%d", ip, port))
	}
	// Return the list of peer addresses
	return peerList, nil
}

func sendExtensionHandshake(conn net.Conn) error {

	// Write the payload
	dict := map[string]any{
		"m": map[string]any{
			"ut_metadata": 1,
		},
	}
	bencode := bencodeEncode(dict)

	msgLen := 2 + len(bencode)
	msg := make([]byte, 4+msgLen)
	binary.BigEndian.PutUint32(msg[0:4], uint32(msgLen))
	msg[4] = 20 // Message ID = 20
	msg[5] = 0  // extension message id
	copy(msg[6:], []byte(bencode))

	_, err := conn.Write(msg)
	return err
}

func readExtensionMessage(conn net.Conn) (msgID, extID byte, header map[string]any, err error) {
	lenBuf := make([]byte, 4)
	if _, err = io.ReadFull(conn, lenBuf); err != nil {
		return
	}
	msgLen := binary.BigEndian.Uint32(lenBuf)

	msgBuf := make([]byte, msgLen)
	if _, err = io.ReadFull(conn, msgBuf); err != nil {
		return
	}

	msgID = msgBuf[0]
	extID = msgBuf[1]
	payload := msgBuf[2:]

	// separate bencoded header and binary data

	payloadStr := string(payload)

	// use your bencodeDecode
	headerDecoded, _, err := decodeBencode(payloadStr)
	if err != nil {
		fmt.Println("Error", err)
	}
	header = headerDecoded.(map[string]any)
	return
}

func sendMetadataRequest(conn net.Conn, pieceIndex int, extID byte) error {

	// Write the payload
	dict := map[string]any{
		"msg_type": 0,
		"piece":    pieceIndex,
	}
	bencode := bencodeEncode(dict)

	msgLen := 2 + len(bencode)
	msg := make([]byte, 4+msgLen)
	binary.BigEndian.PutUint32(msg[0:4], uint32(msgLen))
	msg[4] = 20    // Message ID = 20
	msg[5] = extID // extension message id
	copy(msg[6:], []byte(bencode))

	_, err := conn.Write(msg)
	return err
}

func readMetadataResponse(conn net.Conn) (metadata map[string]any, err error) {
	lenBuf := make([]byte, 4)
	if _, err = io.ReadFull(conn, lenBuf); err != nil {
		return
	}
	msgLen := binary.BigEndian.Uint32(lenBuf)

	msgBuf := make([]byte, msgLen)
	if _, err = io.ReadFull(conn, msgBuf); err != nil {
		return
	}

	// msgID := msgBuf[0]
	// extID := msgBuf[1]
	payload := msgBuf[2:]
	payloadStr := string(payload)
	_, i, err := decodeBencode(payloadStr)
	if err != nil {
		fmt.Println("Error", err)
	}
	metadataDecoded, _, err := decodeBencode(payloadStr[i+1:])
	metadata = metadataDecoded.(map[string]any)

	return
}
